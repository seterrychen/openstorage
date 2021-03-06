package proxy

import (
	"github.com/docker/docker/daemon/graphdriver/overlay"
	"github.com/libopenstorage/openstorage/api"
	"github.com/libopenstorage/openstorage/graph"
)

const (
	Name = "proxy"
	Type = api.DriverType_DRIVER_TYPE_GRAPH
)

func init() {
	graph.Register(Name, overlay.Init)
}
