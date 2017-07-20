package docker

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/quilt/quilt/minion/supervisor/images"
	"github.com/quilt/quilt/minion/ipdef"
	"github.com/quilt/quilt/util"

	log "github.com/Sirupsen/logrus"
	dkc "github.com/fsouza/go-dockerclient"
)

var pullCacheTimeout = time.Minute
var networkTimeout = time.Minute

// ErrNoSuchContainer is the error returned when an operation is requested on a
// non-existent container.
var ErrNoSuchContainer = errors.New("container does not exist")

// A Container as returned by the docker client API.
type Container struct {
	ID      string
	EID     string
	Name    string
	Image   string
	ImageID string
	IP      string
	Mac     string
	Path    string
	Status  string
	Args    []string
	Pid     int
	Env     map[string]string
	Labels  map[string]string
	Created time.Time
}

// ContainerSlice is an alias for []Container to allow for joins
type ContainerSlice []Container

// A Client to the local docker daemon.
type Client struct {
	client
	*sync.Mutex
	imageCache map[string]*cacheEntry
}

type cacheEntry struct {
	sync.Mutex
	expiration time.Time
}

// RunOptions changes the behavior of the Run function.
type RunOptions struct {
	Name              string
	Image             string
	Args              []string
	Labels            map[string]string
	Env               map[string]string
	FilepathToContent map[string]string
	Hostname    string
	IP          string
	NetworkMode string
	DNS         []string
	DNSSearch   []string

	PidMode     string
	Privileged  bool
	VolumesFrom []string

	//Cadvisor Specific
	Binds []string
	ExternalPort string
	HostIP string
	HostPort string
	PublishAllPorts bool
}

type client interface {
	StartContainer(id string, hostConfig *dkc.HostConfig) error
	UploadToContainer(id string, opts dkc.UploadToContainerOptions) error
	DownloadFromContainer(id string, opts dkc.DownloadFromContainerOptions) error
	RemoveContainer(opts dkc.RemoveContainerOptions) error
	BuildImage(opts dkc.BuildImageOptions) error
	PullImage(opts dkc.PullImageOptions, auth dkc.AuthConfiguration) error
	PushImage(opts dkc.PushImageOptions, auth dkc.AuthConfiguration) error
	ListContainers(opts dkc.ListContainersOptions) ([]dkc.APIContainers, error)
	InspectContainer(id string) (*dkc.Container, error)
	InspectImage(id string) (*dkc.Image, error)
	CreateContainer(dkc.CreateContainerOptions) (*dkc.Container, error)
	CreateNetwork(dkc.CreateNetworkOptions) (*dkc.Network, error)
	ListNetworks() ([]dkc.Network, error)
}

// New creates client to the docker daemon.
func New(sock string) Client {
	var client *dkc.Client
	for {
		var err error
		client, err = dkc.NewClient(sock)
		if err != nil {
			log.WithError(err).Warn("Failed to create docker client.")
			time.Sleep(10 * time.Second)
			continue
		}
		break
	}

	return Client{client, &sync.Mutex{}, map[string]*cacheEntry{}}
}

// Run creates and starts a new container in accordance RunOptions.
func (dk Client) Run(opts RunOptions) (string, error) {
	env := []string{}
	for k, v := range opts.Env {
		env = append(env, k+"="+v)
	}

	var portBinding map[dkc.Port][]dkc.PortBinding
	var exposedPorts map[dkc.Port]struct{}

	if opts.Name == images.Monitor {
	   portBinding = map[dkc.Port][]dkc.PortBinding{dkc.Port(opts.ExternalPort): {{HostIP: opts.HostIP, HostPort: opts.HostPort}}}
	   exposedPorts = map[dkc.Port]struct{}{"50000/tcp": {}}
	}

	network_admin_opt := []string{"NET_ADMIN"}
	hc := &dkc.HostConfig{
		NetworkMode: opts.NetworkMode,
		PidMode:     opts.PidMode,
		Privileged:  opts.Privileged,
		VolumesFrom: opts.VolumesFrom,
		DNS:         opts.DNS,
		DNSSearch:   opts.DNSSearch,
		PublishAllPorts: opts.PublishAllPorts,
		PortBindings: portBinding,
		Binds: opts.Binds,
		CapAdd: network_admin_opt,
	}

	var nc *dkc.NetworkingConfig
	if opts.IP != "" {
		nc = &dkc.NetworkingConfig{
			EndpointsConfig: map[string]*dkc.EndpointConfig{
				"quilt": {
					IPAMConfig: &dkc.EndpointIPAMConfig{
						IPv4Address: opts.IP,
					},
				},
			},
		}
	}

	id, err := dk.create(opts.Name, opts.Image, opts.Hostname, opts.Args, exposedPorts, opts.Labels, env,
		opts.FilepathToContent, hc, nc)
	if err != nil {
		return "", err
	}

	if err = dk.StartContainer(id, hc); err != nil {
		dk.RemoveID(id) // Remove the container to avoid a zombie.
		return "", err
	}

	return id, nil
}

// ConfigureNetwork makes a request to docker to create a network running on driver.
func (dk Client) ConfigureNetwork(driver string) error {
	networks, err := dk.ListNetworks()
	if err == nil {
		for _, nw := range networks {
			if nw.Name == driver {
				return nil
			}
		}
	}

	_, err = dk.CreateNetwork(dkc.CreateNetworkOptions{
		Name:   driver,
		Driver: driver,
		IPAM: dkc.IPAMOptions{
			Config: []dkc.IPAMConfig{{
				Subnet:  ipdef.QuiltSubnet.String(),
				Gateway: ipdef.GatewayIP.String(),
			}},
		},
	})

	return err
}

// Remove stops and deletes the container with the given name.
func (dk Client) Remove(name string) error {
	id, err := dk.getID(name)
	if err != nil {
		return err
	}

	return dk.RemoveID(id)
}

// RemoveID stops and deletes the container with the given ID.
func (dk Client) RemoveID(id string) error {
	err := dk.RemoveContainer(dkc.RemoveContainerOptions{ID: id, Force: true})
	if err != nil {
		return err
	}

	return nil
}

// Build builds an image with the given name and Dockerfile, and returns the
// ID of the resulting image.
func (dk Client) Build(name, dockerfile string) (id string, err error) {
	tarBuf, err := util.ToTar("Dockerfile", 0644, dockerfile)
	if err != nil {
		return "", err
	}

	err = dk.BuildImage(dkc.BuildImageOptions{
		NetworkMode:  "host",
		Name:         name,
		InputStream:  tarBuf,
		OutputStream: ioutil.Discard,
	})
	if err != nil {
		return "", err
	}

	img, err := dk.InspectImage(name)
	if err != nil {
		return "", err
	}

	return img.ID, nil
}

// Pull retrieves the given docker image from an image cache.
// The `image` argument can be of the form <repo>, <repo>:<tag>, or
// <repo>:<tag>@<digestFormat>:<digest>.
// If no tag is specified, then the "latest" tag is applied.
func (dk Client) Pull(image string) error {
	repo, tag := dkc.ParseRepositoryTag(image)
	if tag == "" {
		tag = "latest"
	}

	entry := dk.getCacheEntry(repo, tag)
	entry.Lock()
	defer entry.Unlock()

	if time.Now().Before(entry.expiration) {
		return nil
	}

	log.WithField("image", image).Info("Begin image pull")
	opts := dkc.PullImageOptions{Repository: repo,
		Tag:               tag,
		InactivityTimeout: networkTimeout,
	}
	if err := dk.PullImage(opts, dkc.AuthConfiguration{}); err != nil {
		log.WithField("image", image).WithError(err).Error("Failed image pull")
		return err
	}

	entry.expiration = time.Now().Add(pullCacheTimeout)
	log.WithField("image", image).Info("Finish image pull")
	return nil
}

func (dk Client) getCacheEntry(repo, tag string) *cacheEntry {
	dk.Lock()
	defer dk.Unlock()

	key := repo + ":" + tag
	if entry, ok := dk.imageCache[key]; ok {
		return entry
	}
	entry := &cacheEntry{}
	dk.imageCache[key] = entry
	return entry
}

// Push pushes the given image to the registry.
func (dk Client) Push(registry, image string) error {
	repo, tag := dkc.ParseRepositoryTag(image)
	return dk.PushImage(dkc.PushImageOptions{
		Registry: registry,
		Name:     repo,
		Tag:      tag,
	}, dkc.AuthConfiguration{})
}

// List returns a slice of all running containers.  The List can be be filtered with the
// supplied `filters` map.
func (dk Client) List(filters map[string][]string) ([]Container, error) {
	return dk.list(filters, false)
}

func (dk Client) list(filters map[string][]string, all bool) ([]Container, error) {
	opts := dkc.ListContainersOptions{All: all, Filters: filters}
	apics, err := dk.ListContainers(opts)
	if err != nil {
		return nil, err
	}

	var containers []Container
	for _, apic := range apics {
		c, err := dk.Get(apic.ID)
		if err != nil {
			log.WithError(err).Warnf("Failed to inspect container: %s",
				apic.ID)
			continue
		}

		containers = append(containers, c)
	}

	return containers, nil
}

// Get returns a Container corresponding to the supplied ID.
func (dk Client) Get(id string) (Container, error) {
	dkc, err := dk.InspectContainer(id)
	if err != nil {
		return Container{}, err
	}

	env := make(map[string]string)
	for _, value := range dkc.Config.Env {
		e := strings.SplitN(value, "=", 2)
		if len(e) > 1 {
			env[e[0]] = e[1]
		}
	}

	c := Container{
		Name:    dkc.Name,
		ID:      dkc.ID,
		IP:      dkc.NetworkSettings.IPAddress,
		Mac:     dkc.NetworkSettings.MacAddress,
		EID:     dkc.NetworkSettings.EndpointID,
		Image:   dkc.Config.Image,
		ImageID: dkc.Image,
		Path:    dkc.Path,
		Args:    dkc.Args,
		Pid:     dkc.State.Pid,
		Env:     env,
		Labels:  dkc.Config.Labels,
		Status:  dkc.State.Status,
		Created: dkc.Created,
	}

	networks := keys(dkc.NetworkSettings.Networks)
	if len(networks) == 1 {
		config := dkc.NetworkSettings.Networks[networks[0]]
		c.IP = config.IPAddress
		c.Mac = config.MacAddress
		c.EID = config.EndpointID
	} else if len(networks) > 1 {
		log.Warnf("Multiple networks for container: %s", dkc.ID)
	}

	return c, nil
}

func keys(networks map[string]dkc.ContainerNetwork) []string {
	keySet := []string{}
	for key := range networks {
		keySet = append(keySet, key)
	}
	return keySet
}

// IsRunning returns true if the container with the given `name` is running.
func (dk Client) IsRunning(name string) (bool, error) {
	containers, err := dk.List(map[string][]string{
		"name": {name},
	})
	if err != nil {
		return false, err
	}
	return len(containers) != 0, nil
}

func (dk Client) create(name, image, hostname string, args []string, exposedPorts map[dkc.Port]struct{},
	labels map[string]string, env []string, filepathToContent map[string]string,
	hc *dkc.HostConfig, nc *dkc.NetworkingConfig) (string, error) {

	if err := dk.Pull(image); err != nil {
		return "", err
	}

	container, err := dk.CreateContainer(dkc.CreateContainerOptions{
		Name: name,
		Config: &dkc.Config{
			Image:  string(image),
			Hostname: hostname,
			Cmd:    args,
			Labels: labels,
			Env:    env,
			ExposedPorts: exposedPorts},
		HostConfig:       hc,
		NetworkingConfig: nc,
	})
	if err != nil {
		return "", err
	}

	for path, content := range filepathToContent {
		dir := "."
		if filepath.IsAbs(path) {
			dir = "/"
		}

		// We can safely ignore the error returned by `filepath.Rel` because
		// dir can only be `.` or `/`.
		relPath, _ := filepath.Rel(dir, path)
		tarBuf, err := util.ToTar(relPath, 0644, content)
		if err != nil {
			return "", err
		}

		err = dk.UploadToContainer(container.ID, dkc.UploadToContainerOptions{
			InputStream: tarBuf,
			Path:        dir,
		})
		if err != nil {
			return "", err
		}
	}

	return container.ID, nil
}

func (dk Client) getID(name string) (string, error) {
	containers, err := dk.list(map[string][]string{"name": {name}}, true)
	if err != nil {
		return "", err
	}

	if len(containers) > 0 {
		return containers[0].ID, nil
	}

	return "", ErrNoSuchContainer
}

// Get returns the value contained at the given index
func (cs ContainerSlice) Get(ii int) interface{} {
	return cs[ii]
}

// Len returns the number of items in the slice
func (cs ContainerSlice) Len() int {
	return len(cs)
}
