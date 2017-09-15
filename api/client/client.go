package client

//go:generate mockery -name Client

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/quilt/quilt/api"
	"github.com/quilt/quilt/api/pb"
	"github.com/quilt/quilt/connection"
	"github.com/quilt/quilt/db"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	// The timeout for making requests to the daemon once we've connected.
	requestTimeout = time.Minute
)

// Client provides methods to interact with the Quilt daemon.
type Client interface {
	// Close the grpc connection.
	Close() error

	// QueryMachines retrieves the machines tracked by the Quilt daemon.
	QueryMachines() ([]db.Machine, error)

	// QueryContainers retrieves the containers tracked by the Quilt daemon.
	QueryContainers() ([]db.Container, error)

	// QueryEtcd retrieves the etcd information tracked by the Quilt daemon.
	QueryEtcd() ([]db.Etcd, error)

	// QueryConnections retrieves the connection information tracked by the
	// Quilt daemon.
	QueryConnections() ([]db.Connection, error)

	// QueryLoadBalancers retrieves the load balancer information tracked by
	// the Quilt daemon.
	QueryLoadBalancers() ([]db.LoadBalancer, error)

	// QueryBlueprints retrieves blueprint information tracked by the Quilt daemon.
	QueryBlueprints() ([]db.Blueprint, error)

	// QueryCounters retrieves the debugging counters tracked with the Quilt daemon.
	QueryCounters() ([]pb.Counter, error)

	// QueryCounters retrieves the debugging counters tracked by a Quilt minion.
	// Only defined on the daemon.
	QueryMinionCounters(string) ([]pb.Counter, error)

	// QueryImages retrieves the image information tracked by the Quilt daemon.
	QueryImages() ([]db.Image, error)

	// Deploy makes a request to the Quilt daemon to deploy the given deployment.
	// Only defined on the daemon.
	Deploy(deployment string) error

	// Version retrieves the Quilt version of the remote daemon.
	Version() (string, error)
}

// Getter obtains a client connected to the given address.
type Getter func(string, connection.Credentials) (Client, error)

type clientImpl struct {
	pbClient pb.APIClient
	cc       *grpc.ClientConn
}

// New creates a new Quilt client connected to `lAddr`.
func New(lAddr string, creds connection.Credentials) (Client, error) {
	proto, addr, err := api.ParseListenAddress(lAddr)
	if err != nil {
		return nil, err
	}

	cc, err := connection.Client(proto, addr, creds.ClientOpts())
	if err != nil {
		if err == context.DeadlineExceeded {
			err = daemonTimeoutError{
				host:         lAddr,
				connectError: err,
			}
		}
		return nil, err
	}

	pbClient := pb.NewAPIClient(cc)
	return clientImpl{
		pbClient: pbClient,
		cc:       cc,
	}, nil
}

func query(pbClient pb.APIClient, table db.TableType) (interface{}, error) {
	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	reply, err := pbClient.Query(ctx, &pb.DBQuery{Table: string(table)})
	if err != nil {
		return nil, err
	}

	replyBytes := []byte(reply.TableContents)
	switch table {
	case db.MachineTable:
		var machines []db.Machine
		if err := json.Unmarshal(replyBytes, &machines); err != nil {
			return nil, err
		}
		return machines, nil
	case db.ContainerTable:
		var containers []db.Container
		if err := json.Unmarshal(replyBytes, &containers); err != nil {
			return nil, err
		}
		return containers, nil
	case db.EtcdTable:
		var etcds []db.Etcd
		if err := json.Unmarshal(replyBytes, &etcds); err != nil {
			return nil, err
		}
		return etcds, nil
	case db.LoadBalancerTable:
		var loadBalancers []db.LoadBalancer
		if err := json.Unmarshal(replyBytes, &loadBalancers); err != nil {
			return nil, err
		}
		return loadBalancers, nil
	case db.ConnectionTable:
		var connections []db.Connection
		if err := json.Unmarshal(replyBytes, &connections); err != nil {
			return nil, err
		}
		return connections, nil
	case db.BlueprintTable:
		var blueprints []db.Blueprint
		if err := json.Unmarshal(replyBytes, &blueprints); err != nil {
			return nil, err
		}
		return blueprints, nil
	case db.ImageTable:
		var images []db.Image
		if err := json.Unmarshal(replyBytes, &images); err != nil {
			return nil, err
		}
		return images, nil
	default:
		panic(fmt.Sprintf("unsupported table type: %s", table))
	}
}

// Close the grpc connection.
func (c clientImpl) Close() error {
	return c.cc.Close()
}

// QueryMachines retrieves the machines tracked by the Quilt daemon.
func (c clientImpl) QueryMachines() ([]db.Machine, error) {
	rows, err := query(c.pbClient, db.MachineTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.Machine), nil
}

// QueryContainers retrieves the containers tracked by the Quilt daemon.
func (c clientImpl) QueryContainers() ([]db.Container, error) {
	rows, err := query(c.pbClient, db.ContainerTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.Container), nil
}

// QueryEtcd retrieves the etcd information tracked by the Quilt daemon.
func (c clientImpl) QueryEtcd() ([]db.Etcd, error) {
	rows, err := query(c.pbClient, db.EtcdTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.Etcd), nil
}

// QueryConnections retrieves the connection information tracked by the Quilt daemon.
func (c clientImpl) QueryConnections() ([]db.Connection, error) {
	rows, err := query(c.pbClient, db.ConnectionTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.Connection), nil
}

// QueryLoadBalancers retrieves the load balancer information tracked by the
// Quilt daemon.
func (c clientImpl) QueryLoadBalancers() ([]db.LoadBalancer, error) {
	rows, err := query(c.pbClient, db.LoadBalancerTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.LoadBalancer), nil
}

// QueryBlueprints retrieves the blueprint information tracked by the Quilt daemon.
func (c clientImpl) QueryBlueprints() ([]db.Blueprint, error) {
	rows, err := query(c.pbClient, db.BlueprintTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.Blueprint), nil
}

// QueryCounters retrieves the debugging counters tracked with the Quilt daemon.
func (c clientImpl) QueryCounters() ([]pb.Counter, error) {
	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	reply, err := c.pbClient.QueryCounters(ctx, &pb.CountersRequest{})
	if err != nil {
		return nil, err
	}

	return parseCountersReply(reply), nil
}

// QueryCounters retrieves the debugging counters tracked by a Quilt minion.
func (c clientImpl) QueryMinionCounters(host string) ([]pb.Counter, error) {
	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	reply, err := c.pbClient.QueryMinionCounters(ctx,
		&pb.MinionCountersRequest{Host: host})
	if err != nil {
		return nil, err
	}

	return parseCountersReply(reply), nil
}

func parseCountersReply(reply *pb.CountersReply) (counters []pb.Counter) {
	for _, c := range reply.Counters {
		counters = append(counters, *c)
	}
	return counters
}

// QueryImages retrieves the image information tracked by the Quilt daemon.
func (c clientImpl) QueryImages() ([]db.Image, error) {
	rows, err := query(c.pbClient, db.ImageTable)
	if err != nil {
		return nil, err
	}

	return rows.([]db.Image), nil
}

// Deploy makes a request to the Quilt daemon to deploy the given deployment.
func (c clientImpl) Deploy(deployment string) error {
	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	_, err := c.pbClient.Deploy(ctx, &pb.DeployRequest{Deployment: deployment})
	return err
}

// Version retrieves the Quilt version of the remote daemon.
func (c clientImpl) Version() (string, error) {
	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	version, err := c.pbClient.Version(ctx, &pb.VersionRequest{})
	if err != nil {
		return "", err
	}
	return version.Version, nil
}

// daemonTimeoutError represents when we are unable to connect to the Quilt
// daemon because of a timeout.
type daemonTimeoutError struct {
	host         string
	connectError error
}

func (err daemonTimeoutError) Error() string {
	return fmt.Sprintf("Unable to connect to the Quilt daemon at %s: %s. "+
		"Is the quilt daemon running? If not, you can start it with "+
		"`quilt daemon`.", err.host, err.connectError.Error())
}
