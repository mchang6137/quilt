package google

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/quilt/quilt/cloud/acl"
	"github.com/quilt/quilt/cloud/cfg"
	"github.com/quilt/quilt/cloud/google/client"
	"github.com/quilt/quilt/cloud/wait"
	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/join"
	"github.com/quilt/quilt/util"

	log "github.com/Sirupsen/logrus"
	"github.com/satori/go.uuid"
	compute "google.golang.org/api/compute/v1"
)

// DefaultRegion is the preferred location for machines that don't have a
// user specified region preference.
const DefaultRegion = "us-east1-b"

// Zones is the list of supported GCE zones
var Zones = []string{"us-central1-a", "us-east1-b", "europe-west1-b"}

// ephemeralIPName is a constant for what we label NATs with ephemeral IPs in GCE.
const ephemeralIPName = "External NAT"

// floatingIPName is a constant for what we label NATs with floating IPs in GCE.
const floatingIPName = "Floating IP"

const computeBaseURL string = "https://www.googleapis.com/compute/v1/projects"
const (
	// These are the various types of Operations that the GCE API returns
	local = iota
	global
)

// The Provider objects represents a connection to GCE.
type Provider struct {
	client.Client

	imgURL      string // gce url to the VM image
	networkName string // gce identifier for the network
	ipv4Range   string // ipv4 range of the internal network
	intFW       string // gce internal firewall name
	zone        string // gce boot region

	ns string // client namespace
}

// New creates a GCE client.
//
// Providers are differentiated (namespace) by setting the description and
// filtering off of that.
func New(namespace, zone string) (*Provider, error) {
	gce, err := client.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCE client: %s", err.Error())
	}

	prvdr := Provider{
		Client:    gce,
		ns:        namespace,
		ipv4Range: "192.168.0.0/16",
		zone:      zone,
	}
	prvdr.intFW = fmt.Sprintf("%s-internal", prvdr.ns)
	prvdr.imgURL = fmt.Sprintf("%s/%s", computeBaseURL,
		"ubuntu-os-cloud/global/images/ubuntu-1604-xenial-v20170202")
	prvdr.networkName = prvdr.ns

	if err := prvdr.createNetwork(); err != nil {
		log.WithError(err).Debug("failed to start up gce network")
		return nil, err
	}

	return &prvdr, nil
}

// getNetworkConfig extracts the NetworkInterface and AccessConfig from a
// Google instance, and handles checking that these are properly
// defined in the instance.
func getNetworkConfig(inst *compute.Instance) (
	*compute.NetworkInterface, *compute.AccessConfig, error) {
	if len(inst.NetworkInterfaces) != 1 {
		return nil, nil, fmt.Errorf("Google instances are expected to "+
			"have exactly 1 interface; for instance %s, "+
			"found %d", inst.Name, len(inst.NetworkInterfaces))
	}
	iface := inst.NetworkInterfaces[0]
	if len(iface.AccessConfigs) != 1 {
		return nil, nil, fmt.Errorf("Google instances expected to "+
			"have exactly 1 access config (instances "+
			"without a config will not be accessible "+
			"via the public internet, and Google "+
			"does not support more than one config); "+
			"for instance %s, found %d access configs",
			inst.Name, len(iface.AccessConfigs))
	}
	return iface, iface.AccessConfigs[0], nil
}

// List the current machines in the cluster.
func (prvdr *Provider) List() ([]db.Machine, error) {
	var machines []db.Machine
	instances, err := prvdr.ListInstances(prvdr.zone,
		fmt.Sprintf("description eq %s", prvdr.ns))
	if err != nil {
		return nil, err
	}
	for _, instance := range instances.Items {
		machineSplitURL := strings.Split(instance.MachineType, "/")
		mtype := machineSplitURL[len(machineSplitURL)-1]

		iface, accessConfig, err := getNetworkConfig(instance)
		if err != nil {
			return nil, err
		}

		floatingIP := ""
		if accessConfig.Name == floatingIPName {
			floatingIP = accessConfig.NatIP
		}

		machines = append(machines, db.Machine{
			CloudID:    instance.Name,
			PublicIP:   accessConfig.NatIP,
			FloatingIP: floatingIP,
			PrivateIP:  iface.NetworkIP,
			Size:       mtype,
		})
	}
	return machines, nil
}

// Boot blocks while creating instances.
func (prvdr *Provider) Boot(bootSet []db.Machine) error {
	// XXX: should probably have a better clean up routine if an error is encountered
	var names []string
	for _, m := range bootSet {
		if m.Preemptible {
			return errors.New("preemptible instances are not yet implemented")
		}

		name := "quilt-" + uuid.NewV4().String()
		_, err := prvdr.instanceNew(name, m.Size, cfg.Ubuntu(m, ""))
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"id":    m.CloudID,
			}).Error("Failed to start instance.")
			continue
		}
		names = append(names, name)
	}

	return prvdr.wait(names, true)
}

// Stop blocks while deleting the instances.
//
// If an error occurs while deleting, it will finish the ones that have
// successfully started before returning.
func (prvdr *Provider) Stop(machines []db.Machine) error {
	// XXX: should probably have a better clean up routine if an error is encountered
	var names []string
	for _, m := range machines {
		_, err := prvdr.DeleteInstance(prvdr.zone, m.CloudID)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"id":    m.CloudID,
			}).Error("Failed to delete instance.")
			continue
		}
		names = append(names, m.CloudID)
	}
	return prvdr.wait(names, false)
}

// Get() and operationWait() don't always present the same results, so
// Boot() and Stop() must have a special wait to stay in sync with Get().
func (prvdr *Provider) wait(ids []string, shouldLive bool) error {
	return wait.Wait(func() bool {
		machines, err := prvdr.List()
		if err != nil {
			return false
		}

		liveMachines := map[string]struct{}{}
		for _, m := range machines {
			liveMachines[m.CloudID] = struct{}{}
		}

		for _, id := range ids {
			if _, live := liveMachines[id]; live != shouldLive {
				return false
			}
		}
		return true
	})
}

// Blocking wait with a hardcoded timeout.
//
// Waits on operations, the type of which is indicated by 'domain'. All
// operations must be of the same 'domain'
func (prvdr *Provider) operationWait(ops []*compute.Operation, domain int) (err error) {
	if domain != local && domain != global {
		return fmt.Errorf("domain not recognized: %d", domain)
	}

	return util.BackoffWaitFor(func() bool {
		for _, op := range ops {
			var res *compute.Operation
			switch domain {
			case local:
				res, err = prvdr.GetZoneOperation(
					op.Zone, op.Name)
			case global:
				res, err = prvdr.GetGlobalOperation(op.Name)
			}

			if err != nil || res.Status != "DONE" {
				return false
			}
		}
		return true
	}, 30*time.Second, 3*time.Minute)
}

// Create new GCE instance.
//
// Does not check if the operation succeeds.
func (prvdr *Provider) instanceNew(name string, size string,
	cloudConfig string) (*compute.Operation, error) {
	instance := &compute.Instance{
		Name:        name,
		Description: prvdr.ns,
		MachineType: fmt.Sprintf("zones/%s/machineTypes/%s",
			prvdr.zone,
			size),
		Disks: []*compute.AttachedDisk{
			{
				Boot:       true,
				AutoDelete: true,
				InitializeParams: &compute.AttachedDiskInitializeParams{
					SourceImage: prvdr.imgURL,
				},
			},
		},
		NetworkInterfaces: []*compute.NetworkInterface{
			{
				AccessConfigs: []*compute.AccessConfig{
					{
						Type: "ONE_TO_ONE_NAT",
						Name: ephemeralIPName,
					},
				},
				Network: networkURL(prvdr.networkName),
			},
		},
		Metadata: &compute.Metadata{
			Items: []*compute.MetadataItems{
				{
					Key:   "startup-script",
					Value: &cloudConfig,
				},
			},
		},
		Tags: &compute.Tags{
			// Tag the machine with its zone so that we can create zone-scoped
			// firewall rules.
			Items: []string{prvdr.zone},
		},
	}

	return prvdr.InsertInstance(prvdr.zone, instance)
}

// listFirewalls returns the firewalls managed by the cluster. Specifically,
// it returns all firewalls that are attached to the cluster's network, and
// apply to the managed zone.
func (prvdr Provider) listFirewalls() ([]compute.Firewall, error) {
	firewalls, err := prvdr.ListFirewalls()
	if err != nil {
		return nil, fmt.Errorf("list firewalls: %s", err)
	}

	var fws []compute.Firewall
	for _, fw := range firewalls.Items {
		_, nwName := path.Split(fw.Network)
		if nwName != prvdr.networkName || fw.Name == prvdr.intFW {
			continue
		}

		for _, tag := range fw.TargetTags {
			if tag == prvdr.zone {
				fws = append(fws, *fw)
				break
			}
		}
	}

	return fws, nil
}

// parseACLs parses the firewall rules contained in the given firewall into
// `acl.ACL`s.
// parseACLs only handles rules specified in the format that Quilt generates: it
// does not handle all the possible rule strings supported by the Google API.
func (prvdr *Provider) parseACLs(fws []compute.Firewall) (acls []acl.ACL, err error) {
	for _, fw := range fws {
		portACLs, err := parsePorts(fw.Allowed)
		if err != nil {
			return nil, fmt.Errorf("parse ports of %s: %s", fw.Name, err)
		}

		for _, cidrIP := range fw.SourceRanges {
			for _, acl := range portACLs {
				acl.CidrIP = cidrIP
				acls = append(acls, acl)
			}
		}
	}

	return acls, nil
}

func parsePorts(allowed []*compute.FirewallAllowed) (acls []acl.ACL, err error) {
	for _, rule := range allowed {
		for _, portsStr := range rule.Ports {
			portRange, err := parseInts(strings.Split(portsStr, "-"))
			if err != nil {
				return nil, fmt.Errorf("parse ints: %s", err)
			}

			var min, max int
			switch len(portRange) {
			case 1:
				min, max = portRange[0], portRange[0]
			case 2:
				min, max = portRange[0], portRange[1]
			default:
				return nil, fmt.Errorf(
					"unrecognized port format: %s", portsStr)
			}
			acls = append(acls, acl.ACL{MinPort: min, MaxPort: max})
		}
	}
	return acls, nil
}

func parseInts(intStrings []string) (parsed []int, err error) {
	for _, str := range intStrings {
		parsedInt, err := strconv.Atoi(str)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, parsedInt)
	}
	return parsed, nil
}

// SetACLs adds and removes acls in `prvdr` so that it conforms to `acls`.
func (prvdr *Provider) SetACLs(acls []acl.ACL) error {
	fws, err := prvdr.listFirewalls()
	if err != nil {
		return err
	}

	currACLs, err := prvdr.parseACLs(fws)
	if err != nil {
		return fmt.Errorf("parse ACLs: %s", err)
	}

	pair, toAdd, toRemove := join.HashJoin(acl.Slice(acls), acl.Slice(currACLs),
		nil, nil)

	var toSet []acl.ACL
	for _, a := range toAdd {
		toSet = append(toSet, a.(acl.ACL))
	}
	for _, p := range pair {
		toSet = append(toSet, p.L.(acl.ACL))
	}
	for _, a := range toRemove {
		toSet = append(toSet, acl.ACL{
			MinPort: a.(acl.ACL).MinPort,
			MaxPort: a.(acl.ACL).MaxPort,
			CidrIP:  "", // Remove all currently allowed IPs.
		})
	}

	for acl, cidrIPs := range groupACLsByPorts(toSet) {
		fw, err := prvdr.getCreateFirewall(acl.MinPort, acl.MaxPort)
		if err != nil {
			return err
		}

		if reflect.DeepEqual(fw.SourceRanges, cidrIPs) {
			continue
		}

		var op *compute.Operation
		if len(cidrIPs) == 0 {
			log.WithField("ports", fmt.Sprintf(
				"%d-%d", acl.MinPort, acl.MaxPort)).
				Debug("Google: Deleting firewall")
			op, err = prvdr.DeleteFirewall(fw.Name)
			if err != nil {
				return err
			}
		} else {
			log.WithField("ports", fmt.Sprintf(
				"%d-%d", acl.MinPort, acl.MaxPort)).
				WithField("CidrIPs", cidrIPs).
				Debug("Google: Setting ACLs")
			op, err = prvdr.firewallPatch(fw.Name, cidrIPs)
			if err != nil {
				return err
			}
		}
		if err := prvdr.operationWait(
			[]*compute.Operation{op}, global); err != nil {
			return err
		}
	}

	return nil
}

// UpdateFloatingIPs updates IPs of machines by recreating their network interfaces.
func (prvdr *Provider) UpdateFloatingIPs(machines []db.Machine) error {
	for _, m := range machines {
		instance, err := prvdr.GetInstance(prvdr.zone, m.CloudID)
		if err != nil {
			return err
		}

		// Delete existing network interface. It is only possible to assign
		// one access config per instance. Thus, updating GCE Floating IPs
		// is not a seamless, zero-downtime procedure.
		networkInterface, accessConfig, err := getNetworkConfig(instance)
		if err != nil {
			return err
		}
		_, err = prvdr.DeleteAccessConfig(prvdr.zone, m.CloudID,
			accessConfig.Name, networkInterface.Name)
		if err != nil {
			return err
		}

		// Add new network interface.
		_, err = prvdr.AddAccessConfig(prvdr.zone, m.CloudID,
			networkInterface.Name, &compute.AccessConfig{
				Type: "ONE_TO_ONE_NAT",
				Name: floatingIPName,
				// Google will automatically assign a dynamic IP
				// if none is provided (i.e. m.FloatingIP == "").
				NatIP: m.FloatingIP,
			})
		if err != nil {
			return err
		}
	}

	return nil
}

func (prvdr *Provider) getFirewall(name string) (*compute.Firewall, error) {
	list, err := prvdr.ListFirewalls()
	if err != nil {
		return nil, err
	}
	for _, val := range list.Items {
		if val.Name == name {
			return val, nil
		}
	}

	return nil, nil
}

func (prvdr *Provider) getCreateFirewall(minPort int, maxPort int) (
	*compute.Firewall, error) {

	ports := fmt.Sprintf("%d-%d", minPort, maxPort)
	fwName := fmt.Sprintf("%s-%s-%s", prvdr.ns, prvdr.zone, ports)

	if fw, _ := prvdr.getFirewall(fwName); fw != nil {
		return fw, nil
	}

	log.WithField("name", fwName).Debug("Creating firewall")
	op, err := prvdr.insertFirewall(fwName, ports, []string{"127.0.0.1/32"}, true)
	if err != nil {
		return nil, err
	}

	if err := prvdr.operationWait([]*compute.Operation{op}, global); err != nil {
		return nil, err
	}

	return prvdr.getFirewall(fwName)
}

func (prvdr *Provider) networkExists(name string) (bool, error) {
	list, err := prvdr.ListNetworks()
	if err != nil {
		return false, err
	}
	for _, val := range list.Items {
		if val.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// This creates a firewall but does nothing else
func (prvdr *Provider) insertFirewall(name, ports string, sourceRanges []string,
	restrictToZone bool) (*compute.Operation, error) {

	var targetTags []string
	if restrictToZone {
		targetTags = []string{prvdr.zone}
	}

	firewall := &compute.Firewall{
		Name:    name,
		Network: networkURL(prvdr.networkName),
		Allowed: []*compute.FirewallAllowed{
			{
				IPProtocol: "tcp",
				Ports:      []string{ports},
			},
			{
				IPProtocol: "udp",
				Ports:      []string{ports},
			},
			{
				IPProtocol: "icmp",
			},
		},
		SourceRanges: sourceRanges,
		TargetTags:   targetTags,
	}

	return prvdr.InsertFirewall(firewall)
}

func (prvdr *Provider) firewallExists(name string) (bool, error) {
	fw, err := prvdr.getFirewall(name)
	return fw != nil, err
}

// Updates the firewall using PATCH semantics.
//
// The IP addresses must be in CIDR notation.
func (prvdr *Provider) firewallPatch(name string,
	ips []string) (*compute.Operation, error) {
	firewall := &compute.Firewall{
		Name:         name,
		Network:      networkURL(prvdr.networkName),
		SourceRanges: ips,
	}

	return prvdr.PatchFirewall(name, firewall)
}

// Initializes the network for the cluster
func (prvdr *Provider) createNetwork() error {
	exists, err := prvdr.networkExists(prvdr.networkName)
	if err != nil {
		return err
	}

	if exists {
		log.Debug("Network already exists")
		return nil
	}

	log.Debug("Creating network")
	op, err := prvdr.InsertNetwork(&compute.Network{
		Name:      prvdr.networkName,
		IPv4Range: prvdr.ipv4Range,
	})
	if err != nil {
		return err
	}

	err = prvdr.operationWait([]*compute.Operation{op}, global)
	if err != nil {
		return err
	}
	return prvdr.createInternalFirewall()
}

// Initializes the internal firewall for the cluster to allow machines to talk
// on the private network.
func (prvdr *Provider) createInternalFirewall() error {
	var ops []*compute.Operation

	if exists, err := prvdr.firewallExists(prvdr.intFW); err != nil {
		return err
	} else if exists {
		log.Debug("internal firewall already exists")
	} else {
		log.Debug("creating internal firewall")
		op, err := prvdr.insertFirewall(
			prvdr.intFW, "1-65535", []string{prvdr.ipv4Range}, false)
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}

	return prvdr.operationWait(ops, global)
}

func networkURL(networkName string) string {
	return fmt.Sprintf("global/networks/%s", networkName)
}

func groupACLsByPorts(acls []acl.ACL) map[acl.ACL][]string {
	grouped := make(map[acl.ACL][]string)
	for _, a := range acls {
		key := acl.ACL{
			MinPort: a.MinPort,
			MaxPort: a.MaxPort,
		}
		if _, ok := grouped[key]; !ok {
			grouped[key] = nil
		}
		if a.CidrIP != "" {
			grouped[key] = append(grouped[key], a.CidrIP)
		}
	}
	return grouped
}
