package supervisor

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/quilt/quilt/counter"
	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/minion/docker"
	"github.com/quilt/quilt/minion/supervisor/images"

	log "github.com/Sirupsen/logrus"
)

const ovsImage = "quilt/ovs"

// The tunneling protocol to use between machines.
// "stt" and "geneve" are supported.
const tunnelingProtocol = "stt"

var imageMap = map[string]string{
	images.Etcd:          "quay.io/coreos/etcd:v3.0.2",
	images.Ovncontroller: ovsImage,
	images.Ovnnorthd:     ovsImage,
	images.Ovsdb:         ovsImage,
	images.Ovsvswitchd:   ovsImage,
	images.Registry:      "registry:2",
	images.Monitor:       "google/cadvisor:v0.24.1",
}

const etcdHeartbeatInterval = "500"
const etcdElectionTimeout = "5000"

var c = counter.New("Supervisor")

var conn db.Conn
var dk docker.Client
var role db.Role
var oldEtcdIPs []string
var oldIP string

// Run blocks implementing the supervisor module.
func Run(_conn db.Conn, _dk docker.Client, _role db.Role) {
	conn = _conn
	dk = _dk
	role = _role

	imageSet := map[string]struct{}{}
	for _, image := range imageMap {
		imageSet[image] = struct{}{}
	}

	for image := range imageSet {
		go dk.Pull(image)
	}

	switch role {
	case db.Master:
		runMaster()
	case db.Worker:
		runWorker()
	}
}

// run calls out to the Docker client to run the container specified by name.
func run(name string, args ...string) {
	c.Inc("Docker Run " + name)
	isRunning, err := dk.IsRunning(name)
	if err != nil {
		log.WithError(err).Warnf("could not check running status of %s.", name)
		return
	}
	if isRunning {
		return
	}

	ro := docker.RunOptions{
		Name:        name,
		Image:       imageMap[name],
		Args:        args,
		NetworkMode: "host",
		VolumesFrom: []string{"minion"},
	}

	if name == images.Ovsvswitchd {
		ro.Privileged = true
	}
	
	if name == images.Monitor {
		ro.Binds =  []string{"/:/rootfs:ro",
				      "/var/run:/var/run:rw",
				      "/sys:/sys:ro",
				      "/var/lib/docker/:/var/lib/docker:ro"}
		ro.ExternalPort = "50000/tcp"
		ro.HostPort = "50000"
		ro.HostIP = "0.0.0.0"
		ro.PublishAllPorts = true
		ro.Privileged = true
	}

	//DANGER!!! Included here just for research purposes so that we can tag outgoing packets using net_cls labels. This is not possible unless we have the containers in the same privileged namespace. 
	ro.Privileged = true
	
	log.Infof("Start Container: %s", name)
	_, err = dk.Run(ro)
	if err != nil {
		log.WithError(err).Warnf("Failed to run %s.", name)
	}
}

// Remove removes the docker container specified by name.
func Remove(name string) {
	log.WithField("name", name).Info("Removing container")
	err := dk.Remove(name)
	if err != nil && err != docker.ErrNoSuchContainer {
		log.WithError(err).Warnf("Failed to remove %s.", name)
	}
}

func initialClusterString(etcdIPs []string) string {
	var initialCluster []string
	for _, ip := range etcdIPs {
		initialCluster = append(initialCluster,
			fmt.Sprintf("%s=http://%s:2380", nodeName(ip), ip))
	}
	return strings.Join(initialCluster, ",")
}

func nodeName(IP string) string {
	return fmt.Sprintf("master-%s", IP)
}

// execRun() is a global variable so that it can be mocked out by the unit tests.
var execRun = func(name string, arg ...string) error {
	c.Inc(name)
	return exec.Command(name, arg...).Run()
}
