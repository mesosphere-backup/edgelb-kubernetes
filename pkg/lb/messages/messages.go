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

type UnConfigVHostMsg struct {
	VHost config.VHost
}
