package cloud

import (
	"errors"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/quilt/quilt/blueprint"
	"github.com/quilt/quilt/cloud/acl"
	"github.com/quilt/quilt/cloud/amazon"
	"github.com/quilt/quilt/cloud/cfg"
	"github.com/quilt/quilt/cloud/digitalocean"
	"github.com/quilt/quilt/cloud/foreman"
	"github.com/quilt/quilt/cloud/google"
	"github.com/quilt/quilt/cloud/vagrant"
	"github.com/quilt/quilt/connection"
	"github.com/quilt/quilt/counter"
	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/join"
	"github.com/quilt/quilt/util"
)

type provider interface {
	List() ([]db.Machine, error)

	Boot([]db.Machine) error

	Stop([]db.Machine) error

	SetACLs([]acl.ACL) error

	UpdateFloatingIPs([]db.Machine) error
}

var c = counter.New("Cloud")

type cloud struct {
	conn db.Conn

	namespace    string
	providerName db.ProviderName
	region       string
	provider     provider
}

var myIP = util.MyIP
var sleep = time.Sleep

// Run continually checks 'conn' for cloud changes and recreates the cloud as
// needed.
func Run(conn db.Conn, creds connection.Credentials, minionTLSDir string) {
	cfg.MinionTLSDir = minionTLSDir
	foreman.Credentials = creds

	go updateMachineStatuses(conn)

	var ns string
	foreman.Init(conn)
	stop := make(chan struct{})
	for range conn.TriggerTick(60, db.BlueprintTable, db.MachineTable).C {
		newns, _ := conn.GetBlueprintNamespace()
		if newns == ns {
			foreman.RunOnce(conn)
			sleep(5 * time.Second) // Rate-limit the foreman.
			continue
		}

		log.Debugf("Namespace change from \"%s\", to \"%s\".", ns, newns)
		ns = newns

		if ns != "" {
			close(stop)
			stop = make(chan struct{})
			makeClouds(conn, ns, stop)
			foreman.Init(conn)
		}
	}
}

func makeClouds(conn db.Conn, ns string, stop chan struct{}) {
	for _, p := range db.AllProviders {
		for _, r := range validRegions(p) {
			cld, err := newCloud(conn, p, r, ns)
			if err != nil {
				log.WithFields(log.Fields{
					"error":  err,
					"region": cld.String(),
				}).Debug("failed to create cloud provider")
				continue
			}
			go cld.run(stop)
		}
	}
}

func newCloud(conn db.Conn, pName db.ProviderName, region, ns string) (cloud, error) {
	cld := cloud{
		conn:         conn,
		namespace:    ns,
		region:       region,
		providerName: pName,
	}

	var err error
	cld.provider, err = newProvider(pName, ns, region)
	if err != nil {
		return cld, fmt.Errorf("failed to connect: %s", err)
	}
	return cld, nil
}

func (cld cloud) run(stop <-chan struct{}) {
	log.Debugf("Start Cloud %s", cld)

	trigger := cld.conn.TriggerTick(60, db.BlueprintTable, db.MachineTable)
	defer trigger.Stop()

	for {
		select {
		case <-stop:
		case <-trigger.C:
		}

		// In a race between a closed stop and a trigger, choose stop.
		select {
		case <-stop:
			log.Debugf("Stop Cloud %s", cld)
			return
		default:
		}

		cld.runOnce()

		// Somewhat of a crude rate-limit of once every five seconds to
		// avoid stressing out the cloud providers with too many calls.
		sleep(5 * time.Second)
	}
}

func (cld cloud) runOnce() {
	/* Each iteration of this loop does the following:
	 *
	 * - Get the current set of machines and ACLs from the cloud provider.
	 * - Get the current policy from the database.
	 * - Compute a diff.
	 * - Update the cloud provider accordingly.
	 *
	 * Updating the cloud provider may have consequences (creating machines, for
	 * example) that should be reflected in the database.  Therefore, if updates
	 * are necessary, the code loops a second time so that the database can be
	 * updated before the next runOnce() call.
	 */
	for i := 0; i < 2; i++ {
		jr, err := cld.join()
		if err != nil {
			return
		}

		if len(jr.boot) == 0 &&
			len(jr.terminate) == 0 &&
			len(jr.updateIPs) == 0 {
			// ACLs must be processed after Quilt learns about what machines
			// are in the cloud.  If we didn't, inter-machine ACLs could get
			// removed when the Quilt controller restarts, even if there are
			// running cloud machines that still need to communicate.
			cld.syncACLs(jr.acls)
			return
		}

		cld.boot(jr.boot)
		cld.updateCloud(jr.terminate, provider.Stop, "stop")
		cld.updateCloud(jr.updateIPs, provider.UpdateFloatingIPs,
			"update floating IPs")
	}
}

func (cld cloud) boot(machines []db.Machine) {
	// As a defensive measure, we only copy over the fields that the underlying
	// provider should care about instead of passing `machines` to updateCloud
	// directly.
	var cloudMachines []db.Machine
	for _, m := range machines {
		cloudMachines = append(cloudMachines, db.Machine{
			Size:        m.Size,
			DiskSize:    m.DiskSize,
			Preemptible: m.Preemptible,
			SSHKeys:     m.SSHKeys,
			Role:        m.Role,
			Provider:    m.Provider,
			Region:      m.Region,
		})
	}

	setStatuses(cld.conn, machines, db.Booting)
	defer setStatuses(cld.conn, machines, "")
	cld.updateCloud(cloudMachines, provider.Boot, "boot")
}

type machineAction func(provider, []db.Machine) error

func (cld cloud) updateCloud(machines []db.Machine, fn machineAction, action string) {
	if len(machines) == 0 {
		return
	}

	logFields := log.Fields{
		"count":  len(machines),
		"action": action,
		"region": cld.String(),
	}

	c.Inc(action)
	if err := fn(cld.provider, machines); err != nil {
		logFields["error"] = err
		log.WithFields(logFields).Errorf("Failed to update machines.")
	} else {
		log.WithFields(logFields).Infof("Updated machines.")
	}
}

type joinResult struct {
	acls []acl.ACL

	boot      []db.Machine
	terminate []db.Machine
	updateIPs []db.Machine
}

func (cld cloud) join() (joinResult, error) {
	res := joinResult{}

	cloudMachines, err := cld.get()
	if err != nil {
		log.WithError(err).Error("Failed to list machines")
		return res, err
	}

	err = cld.conn.Txn(db.BlueprintTable,
		db.MachineTable).Run(func(view db.Database) error {
		bp, err := view.GetBlueprint()
		if err != nil {
			log.WithError(err).Error("Failed to get blueprint")
			return err
		}

		if cld.namespace != bp.Namespace {
			err := errors.New("namespace change during a cloud run")
			log.WithError(err).Debug("Cloud run abort")
			return err
		}

		var machines []db.Machine
		allMachines := view.SelectFromMachine(nil)
		for _, m := range allMachines {
			if m.Provider == cld.providerName && m.Region == cld.region {
				machines = append(machines, m)
			}
		}

		cloudMachines = getMachineRoles(cloudMachines)

		dbResult := syncDB(cloudMachines, machines)
		res.boot = dbResult.boot
		res.terminate = dbResult.stop
		res.updateIPs = dbResult.updateIPs

		for _, pair := range dbResult.pairs {
			dbm := pair.L.(db.Machine)
			m := pair.R.(db.Machine)

			if m.Role != db.None && m.Role == dbm.Role {
				dbm.CloudID = m.CloudID
			}

			if dbm.PublicIP != m.PublicIP {
				// We're changing the association between a database
				// machine and a cloud machine, so the status is not
				// applicable.
				dbm.Status = ""
			}
			dbm.PublicIP = m.PublicIP
			dbm.PrivateIP = m.PrivateIP

			view.Commit(dbm)
		}

		// Regions with no machines in them should have their ACLs cleared.
		if len(machines) > 0 {
			for acl := range cld.getACLs(bp, allMachines) {
				res.acls = append(res.acls, acl)
			}
		}

		return nil
	})
	return res, err
}

// getACLs generates the set of ACLs that `cld` should have installed. It requires a list
// of every machine in every region so that acls can be generated that allow them to
// communicate with each other.  The machines just in this `cld`s region are not
// sufficient.
func (cld cloud) getACLs(bp db.Blueprint, machines []db.Machine) map[acl.ACL]struct{} {
	aclSet := map[acl.ACL]struct{}{}

	// Always allow traffic from the Quilt controller, so we append local.
	for _, cidr := range append(bp.AdminACL, "local") {
		acl := acl.ACL{
			CidrIP:  cidr,
			MinPort: 1,
			MaxPort: 65535,
		}
		aclSet[acl] = struct{}{}
	}

	/* for _, m := range machines {
		if m.PublicIP != "" {
			// XXX: Look into the minimal set of necessary ports.
			acl := acl.ACL{
				CidrIP:  m.PublicIP + "/32",
				MinPort: 1,
				MaxPort: 65535,
			}
			aclSet[acl] = struct{}{}
		}
	} */

	for _, conn := range bp.Connections {
		if conn.From == blueprint.PublicInternetLabel {
			acl := acl.ACL{
				CidrIP:  "0.0.0.0/0",
				MinPort: conn.MinPort,
				MaxPort: conn.MaxPort,
			}
			aclSet[acl] = struct{}{}
		}
	}

	return aclSet
}

func (cld cloud) syncACLs(unresolvedACLs []acl.ACL) {
	var acls []acl.ACL
	for _, acl := range unresolvedACLs {
		if acl.CidrIP == "local" {
			ip, err := myIP()
			if err != nil {
				log.WithError(err).Error("Failed to retrive local IP.")
				return
			}
			acl.CidrIP = ip + "/32"
		}
		acls = append(acls, acl)
	}

	c.Inc("SetACLs")
	if err := cld.provider.SetACLs(acls); err != nil {
		log.WithError(err).Warnf("Could not update ACLs in %s.", cld)
	}
}

type syncDBResult struct {
	pairs     []join.Pair
	boot      []db.Machine
	stop      []db.Machine
	updateIPs []db.Machine
}

func syncDB(cms []db.Machine, dbms []db.Machine) syncDBResult {
	ret := syncDBResult{}

	pair1, dbmis, cmis := join.Join(dbms, cms, func(l, r interface{}) int {
		dbm := l.(db.Machine)
		m := r.(db.Machine)

		if dbm.CloudID == m.CloudID && dbm.Provider == m.Provider &&
			dbm.Preemptible == m.Preemptible &&
			dbm.Region == m.Region && dbm.Size == m.Size &&
			(m.DiskSize == 0 || dbm.DiskSize == m.DiskSize) &&
			(m.Role == db.None || dbm.Role == m.Role) {
			return 0
		}

		return -1
	})

	pair2, dbmis, cmis := join.Join(dbmis, cmis, func(l, r interface{}) int {
		dbm := l.(db.Machine)
		m := r.(db.Machine)

		if dbm.Provider != m.Provider ||
			dbm.Region != m.Region ||
			dbm.Size != m.Size ||
			dbm.Preemptible != m.Preemptible ||
			(m.DiskSize != 0 && dbm.DiskSize != m.DiskSize) ||
			(m.Role != db.None && dbm.Role != m.Role) {
			return -1
		}

		score := 10
		if dbm.Role != db.None && m.Role != db.None && dbm.Role == m.Role {
			score -= 4
		}
		if dbm.PublicIP == m.PublicIP && dbm.PrivateIP == m.PrivateIP {
			score -= 2
		}
		if dbm.FloatingIP == m.FloatingIP {
			score--
		}
		return score
	})

	for _, cm := range cmis {
		ret.stop = append(ret.stop, cm.(db.Machine))
	}

	for _, dbm := range dbmis {
		m := dbm.(db.Machine)
		ret.boot = append(ret.boot, m)
	}

	for _, pair := range append(pair1, pair2...) {
		dbm := pair.L.(db.Machine)
		m := pair.R.(db.Machine)

		if dbm.CloudID == m.CloudID && dbm.FloatingIP != m.FloatingIP {
			m.FloatingIP = dbm.FloatingIP
			ret.updateIPs = append(ret.updateIPs, m)
		}

		ret.pairs = append(ret.pairs, pair)
	}

	return ret
}

func (cld cloud) get() ([]db.Machine, error) {
	c.Inc("List")

	machines, err := cld.provider.List()
	if err != nil {
		return nil, fmt.Errorf("list %s: %s", cld, err)
	}

	var cloudMachines []db.Machine
	for _, m := range machines {
		m.Provider = cld.providerName
		m.Region = cld.region
		cloudMachines = append(cloudMachines, m)
	}
	return cloudMachines, nil
}

func getMachineRoles(machines []db.Machine) (withRoles []db.Machine) {
	for _, m := range machines {
		m.Role = getMachineRole(m.PublicIP)
		withRoles = append(withRoles, m)
	}
	return withRoles
}

func newProviderImpl(p db.ProviderName, namespace, region string) (provider, error) {
	switch p {
	case db.Amazon:
		return amazon.New(namespace, region)
	case db.Google:
		return google.New(namespace, region)
	case db.DigitalOcean:
		return digitalocean.New(namespace, region)
	case db.Vagrant:
		return vagrant.New(namespace)
	default:
		panic("Unimplemented")
	}
}

func validRegionsImpl(p db.ProviderName) []string {
	switch p {
	case db.Amazon:
		return amazon.Regions
	case db.Google:
		return google.Zones
	case db.DigitalOcean:
		return digitalocean.Regions
	case db.Vagrant:
		return []string{""} // Vagrant has no regions
	default:
		panic("Unimplemented")
	}
}

func (cld cloud) String() string {
	return fmt.Sprintf("%s-%s-%s", cld.providerName, cld.region, cld.namespace)
}

// Stored in variables so they may be mocked out
var newProvider = newProviderImpl
var validRegions = validRegionsImpl
var getMachineRole = foreman.GetMachineRole
