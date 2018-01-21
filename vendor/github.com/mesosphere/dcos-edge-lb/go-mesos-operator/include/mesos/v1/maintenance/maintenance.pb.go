// Code generated by protoc-gen-go. DO NOT EDIT.
// source: github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1/maintenance/maintenance.proto

/*
Package mesos_v1_maintenance is a generated protocol buffer package.

It is generated from these files:
	github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1/maintenance/maintenance.proto

It has these top-level messages:
	Window
	Schedule
	ClusterStatus
*/
package mesos_v1_maintenance

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import mesos_v1 "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1"
import mesos_v1_allocator "github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1/allocator"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// *
// A set of machines scheduled to go into maintenance
// in the same `unavailability`.
type Window struct {
	// Machines affected by this maintenance window.
	MachineIds []*mesos_v1.MachineID `protobuf:"bytes,1,rep,name=machine_ids,json=machineIds" json:"machine_ids,omitempty"`
	// Interval during which this set of machines is expected to be down.
	Unavailability   *mesos_v1.Unavailability `protobuf:"bytes,2,req,name=unavailability" json:"unavailability,omitempty"`
	XXX_unrecognized []byte                   `json:"-"`
}

func (m *Window) Reset()                    { *m = Window{} }
func (m *Window) String() string            { return proto.CompactTextString(m) }
func (*Window) ProtoMessage()               {}
func (*Window) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{0} }

func (m *Window) GetMachineIds() []*mesos_v1.MachineID {
	if m != nil {
		return m.MachineIds
	}
	return nil
}

func (m *Window) GetUnavailability() *mesos_v1.Unavailability {
	if m != nil {
		return m.Unavailability
	}
	return nil
}

// *
// A list of maintenance windows.
// For example, this may represent a rolling restart of agents.
type Schedule struct {
	Windows          []*Window `protobuf:"bytes,1,rep,name=windows" json:"windows,omitempty"`
	XXX_unrecognized []byte    `json:"-"`
}

func (m *Schedule) Reset()                    { *m = Schedule{} }
func (m *Schedule) String() string            { return proto.CompactTextString(m) }
func (*Schedule) ProtoMessage()               {}
func (*Schedule) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{1} }

func (m *Schedule) GetWindows() []*Window {
	if m != nil {
		return m.Windows
	}
	return nil
}

// *
// Represents the maintenance status of each machine in the cluster.
// The lists correspond to the `MachineInfo.Mode` enumeration.
type ClusterStatus struct {
	DrainingMachines []*ClusterStatus_DrainingMachine `protobuf:"bytes,1,rep,name=draining_machines,json=drainingMachines" json:"draining_machines,omitempty"`
	DownMachines     []*mesos_v1.MachineID            `protobuf:"bytes,2,rep,name=down_machines,json=downMachines" json:"down_machines,omitempty"`
	XXX_unrecognized []byte                           `json:"-"`
}

func (m *ClusterStatus) Reset()                    { *m = ClusterStatus{} }
func (m *ClusterStatus) String() string            { return proto.CompactTextString(m) }
func (*ClusterStatus) ProtoMessage()               {}
func (*ClusterStatus) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{2} }

func (m *ClusterStatus) GetDrainingMachines() []*ClusterStatus_DrainingMachine {
	if m != nil {
		return m.DrainingMachines
	}
	return nil
}

func (m *ClusterStatus) GetDownMachines() []*mesos_v1.MachineID {
	if m != nil {
		return m.DownMachines
	}
	return nil
}

type ClusterStatus_DrainingMachine struct {
	Id *mesos_v1.MachineID `protobuf:"bytes,1,req,name=id" json:"id,omitempty"`
	// A list of the most recent responses to inverse offers from frameworks
	// running on this draining machine.
	Statuses         []*mesos_v1_allocator.InverseOfferStatus `protobuf:"bytes,2,rep,name=statuses" json:"statuses,omitempty"`
	XXX_unrecognized []byte                                   `json:"-"`
}

func (m *ClusterStatus_DrainingMachine) Reset()         { *m = ClusterStatus_DrainingMachine{} }
func (m *ClusterStatus_DrainingMachine) String() string { return proto.CompactTextString(m) }
func (*ClusterStatus_DrainingMachine) ProtoMessage()    {}
func (*ClusterStatus_DrainingMachine) Descriptor() ([]byte, []int) {
	return fileDescriptor0, []int{2, 0}
}

func (m *ClusterStatus_DrainingMachine) GetId() *mesos_v1.MachineID {
	if m != nil {
		return m.Id
	}
	return nil
}

func (m *ClusterStatus_DrainingMachine) GetStatuses() []*mesos_v1_allocator.InverseOfferStatus {
	if m != nil {
		return m.Statuses
	}
	return nil
}

func init() {
	proto.RegisterType((*Window)(nil), "mesos.v1.maintenance.Window")
	proto.RegisterType((*Schedule)(nil), "mesos.v1.maintenance.Schedule")
	proto.RegisterType((*ClusterStatus)(nil), "mesos.v1.maintenance.ClusterStatus")
	proto.RegisterType((*ClusterStatus_DrainingMachine)(nil), "mesos.v1.maintenance.ClusterStatus.DrainingMachine")
}

func init() {
	proto.RegisterFile("github.com/mesosphere/dcos-edge-lb/go-mesos-operator/include/mesos/v1/maintenance/maintenance.proto", fileDescriptor0)
}

var fileDescriptor0 = []byte{
	// 381 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xac, 0x52, 0xcd, 0x8b, 0xd3, 0x40,
	0x14, 0xa7, 0x11, 0xd6, 0x65, 0xea, 0xfa, 0x31, 0x7a, 0x08, 0x45, 0x70, 0xa9, 0x20, 0xeb, 0x21,
	0x13, 0x76, 0x15, 0xf1, 0x28, 0xb1, 0x97, 0x1e, 0x44, 0x4d, 0x11, 0x8f, 0x75, 0x3a, 0xf3, 0x9a,
	0x0c, 0x4c, 0x66, 0xc2, 0xcc, 0x24, 0x45, 0x4f, 0xfe, 0x37, 0xfe, 0x9b, 0xd2, 0xc9, 0x57, 0x5b,
	0xb2, 0xb7, 0xde, 0x1e, 0xf9, 0x7d, 0xbd, 0xf7, 0xcb, 0x20, 0x96, 0x09, 0x97, 0x57, 0x1b, 0xc2,
	0x74, 0x11, 0x17, 0x60, 0xb5, 0x2d, 0x73, 0x30, 0x10, 0x73, 0xa6, 0x6d, 0x04, 0x3c, 0x83, 0x48,
	0x6e, 0xe2, 0x4c, 0x47, 0x1e, 0x8a, 0x74, 0x09, 0x86, 0x3a, 0x6d, 0x62, 0xa1, 0x98, 0xac, 0x38,
	0x34, 0x8a, 0xb8, 0xbe, 0x8d, 0x0b, 0x2a, 0x94, 0x03, 0x45, 0x15, 0x83, 0xc3, 0x99, 0x94, 0x46,
	0x3b, 0x8d, 0x5f, 0x78, 0x1e, 0xa9, 0x6f, 0xc9, 0x01, 0x36, 0xfb, 0x7e, 0xa6, 0x68, 0xef, 0xed,
	0x83, 0x66, 0xeb, 0xf3, 0x58, 0x52, 0x29, 0x35, 0xf3, 0x50, 0x3f, 0x35, 0x01, 0xf3, 0xbf, 0x13,
	0x74, 0xf1, 0x53, 0x28, 0xae, 0x77, 0xf8, 0x3d, 0x9a, 0x16, 0x94, 0xe5, 0x42, 0xc1, 0x5a, 0x70,
	0x1b, 0x4e, 0xae, 0x1f, 0xdc, 0x4c, 0xef, 0x9e, 0x93, 0xfe, 0xd4, 0x2f, 0x0d, 0xb8, 0x5c, 0xa4,
	0xa8, 0xe5, 0x2d, 0xb9, 0xc5, 0x9f, 0xd0, 0xe3, 0x4a, 0xd1, 0x9a, 0x0a, 0x49, 0x37, 0x42, 0x0a,
	0xf7, 0x3b, 0x0c, 0xae, 0x83, 0x9b, 0xe9, 0x5d, 0x38, 0x08, 0x7f, 0x1c, 0xe1, 0xe9, 0x09, 0x7f,
	0x9e, 0xa0, 0xcb, 0x15, 0xcb, 0x81, 0x57, 0x12, 0xf0, 0x07, 0xf4, 0x70, 0xe7, 0xb7, 0xe9, 0xf2,
	0x5f, 0x92, 0xb1, 0xaa, 0x49, 0xb3, 0x72, 0xda, 0x91, 0xe7, 0xff, 0x02, 0x74, 0xf5, 0x59, 0x56,
	0xd6, 0x81, 0x59, 0x39, 0xea, 0x2a, 0x8b, 0x7f, 0xa1, 0x67, 0xdc, 0x50, 0xa1, 0x84, 0xca, 0xd6,
	0xed, 0xba, 0x9d, 0xe7, 0xbb, 0x71, 0xcf, 0x23, 0x3d, 0x59, 0xb4, 0xe2, 0xf6, 0xea, 0xf4, 0x29,
	0x3f, 0xfe, 0x60, 0xf1, 0x47, 0x74, 0xc5, 0xf5, 0x4e, 0x0d, 0xee, 0xc1, 0xfd, 0x8d, 0x3d, 0xda,
	0x33, 0x3b, 0xe5, 0xec, 0x0f, 0x7a, 0x72, 0x62, 0x8f, 0x5f, 0xa3, 0x40, 0xf0, 0x70, 0xe2, 0xab,
	0x1b, 0x75, 0x08, 0x04, 0xc7, 0x09, 0xba, 0xb4, 0x7e, 0xbb, 0x3e, 0xec, 0xcd, 0x40, 0x1d, 0xfe,
	0xec, 0x52, 0xd5, 0x60, 0x2c, 0x7c, 0xdd, 0x6e, 0xbb, 0x6b, 0xd2, 0x5e, 0x97, 0xbc, 0x45, 0xaf,
	0xb4, 0xc9, 0x08, 0x2d, 0x29, 0xcb, 0x61, 0xb4, 0x88, 0xe4, 0xe2, 0xdb, 0xfe, 0x69, 0xd8, 0xff,
	0x01, 0x00, 0x00, 0xff, 0xff, 0x06, 0x86, 0xd4, 0x11, 0x4b, 0x03, 0x00, 0x00,
}
