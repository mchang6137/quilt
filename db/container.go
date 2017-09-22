package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/quilt/quilt/util"
)

// A Container row is created for each container specified by the policy.  Each row will
// eventually be instantiated within its corresponding cluster.
// Used only by the minion.
type Container struct {
	ID int `json:"-"`

	IP                string            `json:",omitempty"`
	Minion            string            `json:",omitempty"`
	EndpointID        string            `json:",omitempty"`
	BlueprintID       string            `json:",omitempty"`
	DockerID          string            `json:",omitempty"`
	Status            string            `json:",omitempty"`
	Command           []string          `json:",omitempty"`
	Env               map[string]string `json:",omitempty"`
	FilepathToContent map[string]string `json:",omitempty"`
	Hostname          string            `json:",omitempty"`
	Created           time.Time         `json:","`

	Image      string `json:",omitempty"`
	ImageID    string `json:",omitempty"`
	Dockerfile string `json:"-"`
}

// ContainerSlice is an alias for []Container to allow for joins
type ContainerSlice []Container

// InsertContainer creates a new container row and inserts it into the database.
func (db Database) InsertContainer() Container {
	result := Container{ID: db.nextID()}
	db.insert(result)
	return result
}

// SelectFromContainer gets all containers in the database that satisfy 'check'.
func (db Database) SelectFromContainer(check func(Container) bool) []Container {
	var result []Container
	for _, row := range db.selectRows(ContainerTable) {
		if check == nil || check(row.(Container)) {
			result = append(result, row.(Container))
		}
	}

	return result
}

// SelectFromContainer gets all containers in the database that satisfy the 'check'.
func (conn Conn) SelectFromContainer(check func(Container) bool) []Container {
	var containers []Container
	conn.Txn(ContainerTable).Run(func(view Database) error {
		containers = view.SelectFromContainer(check)
		return nil
	})
	return containers
}

func (c Container) getID() int {
	return c.ID
}

func (c Container) String() string {
	cmdStr := strings.Join(append([]string{"run", c.Image}, c.Command...), " ")
	tags := []string{cmdStr}

	if c.ImageID != "" {
		id := util.ShortUUID(c.ImageID)
		tags = append(tags, fmt.Sprintf("ImageID: %s", id))
	}

	if c.DockerID != "" {
		id := util.ShortUUID(c.DockerID)
		tags = append(tags, fmt.Sprintf("DockerID: %s", id))
	}

	if c.Minion != "" {
		tags = append(tags, fmt.Sprintf("Minion: %s", c.Minion))
	}

	if c.BlueprintID != "" {
		tags = append(tags, fmt.Sprintf("BlueprintID: %s", c.BlueprintID))
	}

	if c.IP != "" {
		tags = append(tags, fmt.Sprintf("IP: %s", c.IP))
	}

	if c.Hostname != "" {
		tags = append(tags, fmt.Sprintf("Hostname: %s", c.Hostname))
	}

	if len(c.Env) > 0 {
		tags = append(tags, fmt.Sprintf("Env: %s", c.Env))
	}

	if len(c.Status) > 0 {
		tags = append(tags, fmt.Sprintf("Status: %s", c.Status))
	}

	if !c.Created.IsZero() {
		tags = append(tags, fmt.Sprintf("Created: %s", c.Created.String()))
	}

	return fmt.Sprintf("Container-%d{%s}", c.ID, strings.Join(tags, ", "))
}

func (c Container) less(r row) bool {
	return c.BlueprintID < r.(Container).BlueprintID
}

// Less implements less than for sort.Interface.
func (cs ContainerSlice) Less(i, j int) bool {
	return cs[i].less(cs[j])
}

// Swap implements swapping for sort.Interface.
func (cs ContainerSlice) Swap(i, j int) {
	cs[i], cs[j] = cs[j], cs[i]
}

// Get returns the value contained at the given index
func (cs ContainerSlice) Get(ii int) interface{} {
	return cs[ii]
}

// Len returns the number of items in the slice
func (cs ContainerSlice) Len() int {
	return len(cs)
}
