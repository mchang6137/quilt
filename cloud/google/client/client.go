//go:generate mockery -name=Client

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2/google"

	compute "google.golang.org/api/compute/v1"

	"github.com/quilt/quilt/counter"
	"github.com/quilt/quilt/util"
)

// A Client for Google's API. Used for unit testing.
type Client interface {
	GetInstance(zone, id string) (*compute.Instance, error)
	ListInstances(zone, filter string) (*compute.InstanceList, error)
	InsertInstance(zone string, instance *compute.Instance) (
		*compute.Operation, error)
	DeleteInstance(zone, operation string) (*compute.Operation, error)
	AddAccessConfig(zone, instance, networkInterface string,
		accessConfig *compute.AccessConfig) (*compute.Operation, error)
	DeleteAccessConfig(zone, instance, accessConfig,
		networkInterface string) (*compute.Operation, error)
	GetZoneOperation(zone, operation string) (*compute.Operation, error)
	GetGlobalOperation(operation string) (*compute.Operation, error)
	ListFirewalls() (*compute.FirewallList, error)
	InsertFirewall(firewall *compute.Firewall) (
		*compute.Operation, error)
	PatchFirewall(name string, firewall *compute.Firewall) (
		*compute.Operation, error)
	DeleteFirewall(firewall string) (*compute.Operation, error)
	ListNetworks() (*compute.NetworkList, error)
	InsertNetwork(network *compute.Network) (
		*compute.Operation, error)
}

type client struct {
	gce    *compute.Service
	projID string
}

var c = counter.New("Google")

// New creates a new Google client.
func New() (Client, error) {
	c.Inc("New Client")

	configPath := filepath.Join(os.Getenv("HOME"), ".gce", "quilt.json")
	configStr, err := util.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	service, err := newComputeService(configStr)
	if err != nil {
		return nil, err
	}

	projID, err := getProjectID(configStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get project ID: %s", err)
	}

	return &client{gce: service, projID: projID}, nil
}

func newComputeService(configStr string) (*compute.Service, error) {
	jwtConfig, err := google.JWTConfigFromJSON(
		[]byte(configStr), compute.ComputeScope)
	if err != nil {
		return nil, err
	}

	return compute.New(jwtConfig.Client(context.Background()))
}

const projectIDKey = "project_id"

func getProjectID(configStr string) (string, error) {
	configFields := map[string]string{}
	if err := json.Unmarshal([]byte(configStr), &configFields); err != nil {
		return "", err
	}

	projID, ok := configFields[projectIDKey]
	if !ok {
		return "", fmt.Errorf("missing field: %s", projectIDKey)
	}

	return projID, nil
}

func (ci *client) GetInstance(zone, id string) (*compute.Instance, error) {
	c.Inc("Get Instance")
	return ci.gce.Instances.Get(ci.projID, zone, id).Do()
}

func (ci *client) ListInstances(zone, filter string) (*compute.InstanceList, error) {
	c.Inc("List Instances")
	call := ci.gce.Instances.List(ci.projID, zone)
	if filter != "" {
		call = call.Filter(filter)
	}

	return call.Do()
}

func (ci *client) InsertInstance(zone string, instance *compute.Instance) (
	*compute.Operation, error) {
	c.Inc("Insert Instance")
	return ci.gce.Instances.Insert(ci.projID, zone, instance).Do()
}

func (ci *client) DeleteInstance(zone, instance string) (*compute.Operation,
	error) {
	return ci.gce.Instances.Delete(ci.projID, zone, instance).Do()
}

func (ci *client) AddAccessConfig(zone, instance, networkInterface string,
	accessConfig *compute.AccessConfig) (*compute.Operation, error) {
	c.Inc("Add Access Config")
	return ci.gce.Instances.AddAccessConfig(ci.projID, zone, instance,
		networkInterface, accessConfig).Do()
}

func (ci *client) DeleteAccessConfig(zone, instance, accessConfig,
	networkInterface string) (*compute.Operation, error) {
	c.Inc("Delete Access Config")
	return ci.gce.Instances.DeleteAccessConfig(ci.projID, zone, instance,
		accessConfig, networkInterface).Do()
}

func (ci *client) GetZoneOperation(zone, operation string) (
	*compute.Operation, error) {
	c.Inc("Get Zone Op")
	return ci.gce.ZoneOperations.Get(ci.projID, zone, operation).Do()
}

func (ci *client) GetGlobalOperation(operation string) (*compute.Operation,
	error) {
	c.Inc("Get Global Op")
	return ci.gce.GlobalOperations.Get(ci.projID, operation).Do()
}

func (ci *client) ListFirewalls() (*compute.FirewallList, error) {
	c.Inc("List Firewalls")
	return ci.gce.Firewalls.List(ci.projID).Do()
}

func (ci *client) InsertFirewall(firewall *compute.Firewall) (
	*compute.Operation, error) {
	c.Inc("Insert Firewall")
	return ci.gce.Firewalls.Insert(ci.projID, firewall).Do()
}

func (ci *client) PatchFirewall(name string, firewall *compute.Firewall) (
	*compute.Operation, error) {
	c.Inc("Patch Firewall")
	return ci.gce.Firewalls.Patch(ci.projID, name, firewall).Do()
}

func (ci *client) DeleteFirewall(firewall string) (
	*compute.Operation, error) {
	c.Inc("Delete Firewall")
	return ci.gce.Firewalls.Delete(ci.projID, firewall).Do()
}

func (ci *client) ListNetworks() (*compute.NetworkList, error) {
	c.Inc("List Networks")
	return ci.gce.Networks.List(ci.projID).Do()
}

func (ci *client) InsertNetwork(network *compute.Network) (
	*compute.Operation, error) {
	c.Inc("Insert Network")
	return ci.gce.Networks.Insert(ci.projID, network).Do()
}
