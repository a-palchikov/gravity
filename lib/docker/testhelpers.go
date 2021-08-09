package docker

import (
	"github.com/gravitational/gravity/lib/docker/test"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/utils"

	"github.com/sirupsen/logrus"
	"gopkg.in/check.v1"
)

var TestRegistryFactory = &testRegistryImpl{}

// newTestRegistry returns a new started docker registry
func newTestRegistry(dir string, s *Synchronizer, c *check.C) *testRegistry {
	config := BasicConfiguration("127.0.0.1:0", dir)
	r, err := NewRegistry(config)
	c.Assert(err, check.IsNil)
	c.Assert(r.Start(), check.IsNil)
	return &testRegistry{
		r:   r,
		dir: dir,
		info: RegistryInfo{
			Address:  r.Addr(),
			Protocol: "http",
		},
		helper: s,
	}
}

func (r *testRegistry) Close() error {
	return r.r.Close()
}

func (r *testRegistry) URL() string {
	return r.info.URL()
}

func (r *testRegistry) Addr() string {
	return r.r.Addr()
}

// Push pushes the set of images to the underlying registry
func (r *testRegistry) Push(c *check.C, images ...loc.DockerImage) {
	for _, image := range images {
		c.Assert(r.helper.Push(image.String(), r.r.Addr()), check.IsNil)
	}
}

// registry is a test docker registry instance
type testRegistry struct {
	dir    string
	r      *Registry
	info   RegistryInfo
	helper *Synchronizer
}

func (r *testRegistryImpl) New(dir string, c *check.C) test.Registry {
	client, err := NewClientFromEnv()
	c.Assert(err, check.IsNil)
	return newTestRegistry(dir, NewSynchronizer(logrus.New(), client, utils.DiscardProgress), c)
}

type testRegistryImpl struct{}
