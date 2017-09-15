package registry

import (
	"errors"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/minion/docker"
)

/*
The registry submodule builds custom Dockerfiles. When a custom Dockerfile is
deployed in a blueprint (e.g.`new Container(new Image("name", "dk"))`), a couple
things happen:
1) On the leader, the engine reads the custom images from the Containers in the
blueprint, and writes them to the Image table.
2) The registry submodule builds the images in the Image table, and updates
their image ID with the ID of the built image.
3) The scheduler schedules containers for which the image has been built.
When scheduling Containers with custom images, it modifies the image to
be pointed at the registry running on the leader. A side effect of this is that
if the leader dies, the scheduler updates the image names in etcd, and the workers
restart containers running the custom image.
4) The workers pull and run the image just like any other image.
*/

// Run builds Docker images according to the Image table if the minion's Role is
// Master, and does nothing otherwise.
func Run(conn db.Conn, dk docker.Client) {
	if conn.MinionSelf().Role != db.Master {
		return
	}

	bootWait()
	for range conn.TriggerTick(30, db.ImageTable).C {
		syncImages(conn, dk)
	}
}

// syncImages checks the Image table for any images that have not yet been
// built, and builds them.
func syncImages(conn db.Conn, dk docker.Client) {
	var toBuild []db.Image
	conn.Txn(db.ImageTable).Run(func(view db.Database) error {
		toBuild = view.SelectFromImage(func(img db.Image) bool {
			return img.Status != db.Built
		})
		return nil
	})

	for _, img := range toBuild {
		img.Status = db.Building
		writeImage(conn, img)

		id, err := updateRegistry(dk, img)
		if err != nil {
			img.Status = "" // Unset the building status.
			writeImage(conn, img)

			log.WithError(err).WithField("image", img.Name).
				Error("Failed to update registry")
			continue
		}

		img.DockerID = id
		img.Status = db.Built
		writeImage(conn, img)
	}
}

func updateRegistry(dk docker.Client, img db.Image) (string, error) {
	registryImg := "localhost:5000/" + img.Name
	id, err := dk.Build(registryImg, img.Dockerfile, false)
	if err == nil {
		err = dk.Push("localhost:5000", registryImg)
	}
	return id, err
}

// writeImage updates the attributes of the image committed to the database that
// has the same Name and Dockerfile.
func writeImage(conn db.Conn, img db.Image) {
	err := conn.Txn(db.ImageTable).Run(
		func(view db.Database) error {
			dbImg, err := getImageHandle(view, img)
			if err == nil {
				img.ID = dbImg.ID
				view.Commit(img)
			}
			return err
		},
	)
	if err != nil {
		log.WithError(err).WithField("image", img).Warn("Failed to write image")
	}
}

func getImageHandle(view db.Database, ref db.Image) (db.Image, error) {
	matchingImages := view.SelectFromImage(func(img db.Image) bool {
		return img.Dockerfile == ref.Dockerfile && img.Name == ref.Name
	})
	switch len(matchingImages) {
	case 0:
		return db.Image{}, errors.New("no matching images")
	case 1:
		return matchingImages[0], nil
	default:
		return db.Image{}, errors.New("multiple matching images")
	}
}

// bootWait blocks until the registry is ready to be pushed to.
func bootWait() {
	for {
		_, err := http.Get("http://localhost:5000")
		if err != nil {
			log.WithError(err).Debug("Registry not up yet")
		} else {
			break
		}

		time.Sleep(500 * time.Millisecond)
	}
}
