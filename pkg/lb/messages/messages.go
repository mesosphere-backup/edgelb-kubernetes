package messages

import (
	"edgelb-k8s/pkg/lb/config"
)

type SyncMsg struct{}

type ConfigVHostsMsg struct {
	VHosts []config.VHost
}

type ConfigVHostMsg struct {
	VHost config.VHost
}

// Note that the `RemoveVHostMsg` is identical to the `ConfigVHostsMsg`. The
// semantic here is that the controller presents the load-balancer with all the
// existing VHosts, minus the VHost that got removed.
type RemoveVHostMsg struct {
	VHosts []config.VHost
}

type UnConfigVHostMsg struct {
	VHost config.VHost
}
