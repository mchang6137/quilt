// Code generated by mockery v1.0.0
package mocks

import db "github.com/quilt/quilt/db"
import mock "github.com/stretchr/testify/mock"
import pb "github.com/quilt/quilt/api/pb"

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// Close provides a mock function with given fields:
func (_m *Client) Close() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Deploy provides a mock function with given fields: deployment
func (_m *Client) Deploy(deployment string) error {
	ret := _m.Called(deployment)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(deployment)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// QueryBlueprints provides a mock function with given fields:
func (_m *Client) QueryBlueprints() ([]db.Blueprint, error) {
	ret := _m.Called()

	var r0 []db.Blueprint
	if rf, ok := ret.Get(0).(func() []db.Blueprint); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]db.Blueprint)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// QueryConnections provides a mock function with given fields:
func (_m *Client) QueryConnections() ([]db.Connection, error) {
	ret := _m.Called()

	var r0 []db.Connection
	if rf, ok := ret.Get(0).(func() []db.Connection); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]db.Connection)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// QueryContainers provides a mock function with given fields:
func (_m *Client) QueryContainers() ([]db.Container, error) {
	ret := _m.Called()

	var r0 []db.Container
	if rf, ok := ret.Get(0).(func() []db.Container); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]db.Container)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// QueryCounters provides a mock function with given fields:
func (_m *Client) QueryCounters() ([]pb.Counter, error) {
	ret := _m.Called()

	var r0 []pb.Counter
	if rf, ok := ret.Get(0).(func() []pb.Counter); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]pb.Counter)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// QueryEtcd provides a mock function with given fields:
func (_m *Client) QueryEtcd() ([]db.Etcd, error) {
	ret := _m.Called()

	var r0 []db.Etcd
	if rf, ok := ret.Get(0).(func() []db.Etcd); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]db.Etcd)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// QueryImages provides a mock function with given fields:
func (_m *Client) QueryImages() ([]db.Image, error) {
	ret := _m.Called()

	var r0 []db.Image
	if rf, ok := ret.Get(0).(func() []db.Image); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]db.Image)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// QueryLoadBalancers provides a mock function with given fields:
func (_m *Client) QueryLoadBalancers() ([]db.LoadBalancer, error) {
	ret := _m.Called()

	var r0 []db.LoadBalancer
	if rf, ok := ret.Get(0).(func() []db.LoadBalancer); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]db.LoadBalancer)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// QueryMachines provides a mock function with given fields:
func (_m *Client) QueryMachines() ([]db.Machine, error) {
	ret := _m.Called()

	var r0 []db.Machine
	if rf, ok := ret.Get(0).(func() []db.Machine); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]db.Machine)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// QueryMinionCounters provides a mock function with given fields: _a0
func (_m *Client) QueryMinionCounters(_a0 string) ([]pb.Counter, error) {
	ret := _m.Called(_a0)

	var r0 []pb.Counter
	if rf, ok := ret.Get(0).(func(string) []pb.Counter); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]pb.Counter)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Version provides a mock function with given fields:
func (_m *Client) Version() (string, error) {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
