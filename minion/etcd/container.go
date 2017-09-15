package etcd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/join"
	"github.com/quilt/quilt/util"

	log "github.com/Sirupsen/logrus"
)

const containerPath = "/containers"

func runContainer(conn db.Conn, store Store) {
	etcdWatch := store.Watch(containerPath, 1*time.Second)
	trigg := conn.TriggerTick(60, db.ContainerTable)
	for range util.JoinNotifiers(trigg.C, etcdWatch) {
		if err := runContainerOnce(conn, store); err != nil {
			log.WithError(err).Warn("Failed to sync containers with Etcd.")
		}
	}
}

func runContainerOnce(conn db.Conn, store Store) error {
	etcdStr, err := readEtcdNode(store, containerPath)
	if err != nil {
		return fmt.Errorf("etcd read error: %s", err)
	}

	if conn.EtcdLeader() {
		c.Inc("Run Container Leader")
		return updateLeader(conn, store, etcdStr)
	}

	c.Inc("Run Container Worker")
	updateNonLeader(conn, etcdStr)
	return nil
}

func updateLeader(conn db.Conn, store Store, etcdStr string) error {
	self := conn.MinionSelf()
	myIP := self.PrivateIP

	dbcs := conn.SelectFromContainer(func(dbc db.Container) bool {
		return dbc.Minion != "" && dbc.IP != ""
	})
	for i := range dbcs {
		if dbcs[i].Dockerfile == "" {
			continue
		}
		dbcs[i].Image = myIP + ":5000/" + dbcs[i].Image
	}

	err := writeEtcdSlice(store, containerPath, etcdStr, db.ContainerSlice(dbcs))
	if err != nil {
		return fmt.Errorf("etcd write error: %s", err)
	}

	return nil
}

func updateNonLeader(conn db.Conn, etcdStr string) {
	self := conn.MinionSelf()

	var rawEtcdDBCs, etcdDBCs []db.Container
	json.Unmarshal([]byte(etcdStr), &rawEtcdDBCs)
	for _, dbc := range rawEtcdDBCs {
		if self.Role == db.Master || dbc.Minion == self.PrivateIP {
			etcdDBCs = append(etcdDBCs, dbc)
		}
	}

	conn.Txn(db.ContainerTable).Run(func(view db.Database) error {
		joinContainers(view, etcdDBCs)
		return nil
	})
}

func joinContainers(view db.Database, etcdDBCs []db.Container) {
	// The join contains only those fields that require restart of the container.
	key := func(iface interface{}) interface{} {
		dbc := iface.(db.Container)

		return struct {
			Hostname          string
			IP                string
			StitchID          string
			Image             string
			ImageID           string
			Command           string
			Env               string
			FilepathToContent string
		}{
			Hostname:          dbc.Hostname,
			IP:                dbc.IP,
			StitchID:          dbc.StitchID,
			Image:             dbc.Image,
			ImageID:           dbc.ImageID,
			Command:           fmt.Sprintf("%v", dbc.Command),
			Env:               util.MapAsString(dbc.Env),
			FilepathToContent: util.MapAsString(dbc.FilepathToContent),
		}
	}

	pairs, dbcIfaces, etcdDBCIfaces := join.HashJoin(
		db.ContainerSlice(view.SelectFromContainer(nil)),
		db.ContainerSlice(etcdDBCs), key, key)

	for _, dbcI := range dbcIfaces {
		view.Remove(dbcI.(db.Container))
	}

	for _, edbc := range etcdDBCIfaces {
		dbc := view.InsertContainer()
		pairs = append(pairs, join.Pair{L: dbc, R: edbc})
	}

	for _, pair := range pairs {
		dbc := pair.L.(db.Container)
		edbc := pair.R.(db.Container)

		dbc.IP = edbc.IP
		dbc.Hostname = edbc.Hostname
		dbc.Minion = edbc.Minion
		dbc.StitchID = edbc.StitchID
		dbc.Image = edbc.Image
		dbc.ImageID = edbc.ImageID
		dbc.Command = edbc.Command
		dbc.Env = edbc.Env
		dbc.FilepathToContent = edbc.FilepathToContent
		dbc.Hostname = edbc.Hostname
		view.Commit(dbc)
	}
}
