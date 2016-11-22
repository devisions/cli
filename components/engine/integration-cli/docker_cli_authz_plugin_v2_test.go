// +build !windows

package main

import (
	"fmt"
	"strings"

	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
)

var (
	authzPluginName            = "riyaz/authz-no-volume-plugin"
	authzPluginTag             = "latest"
	authzPluginNameWithTag     = authzPluginName + ":" + authzPluginTag
	authzPluginBadManifestName = "riyaz/authz-plugin-bad-manifest"
	nonexistentAuthzPluginName = "riyaz/nonexistent-authz-plugin"
)

func init() {
	check.Suite(&DockerAuthzV2Suite{
		ds: &DockerSuite{},
	})
}

type DockerAuthzV2Suite struct {
	ds *DockerSuite
	d  *Daemon
}

func (s *DockerAuthzV2Suite) SetUpTest(c *check.C) {
	testRequires(c, DaemonIsLinux, Network)
	s.d = NewDaemon(c)
	c.Assert(s.d.Start(), check.IsNil)
}

func (s *DockerAuthzV2Suite) TearDownTest(c *check.C) {
	if s.d != nil {
		s.d.Stop()
		s.ds.TearDownTest(c)
	}
}

func (s *DockerAuthzV2Suite) TestAuthZPluginAllowNonVolumeRequest(c *check.C) {
	testRequires(c, IsAmd64)
	// Install authz plugin
	_, err := s.d.Cmd("plugin", "install", "--grant-all-permissions", authzPluginNameWithTag)
	c.Assert(err, checker.IsNil)
	// start the daemon with the plugin and load busybox, --net=none build fails otherwise
	// because it needs to pull busybox
	c.Assert(s.d.Restart("--authorization-plugin="+authzPluginNameWithTag), check.IsNil)
	c.Assert(s.d.LoadBusybox(), check.IsNil)

	// defer disabling the plugin
	defer func() {
		c.Assert(s.d.Restart(), check.IsNil)
		_, err = s.d.Cmd("plugin", "disable", authzPluginNameWithTag)
		c.Assert(err, checker.IsNil)
		_, err = s.d.Cmd("plugin", "rm", authzPluginNameWithTag)
		c.Assert(err, checker.IsNil)
	}()

	// Ensure docker run command and accompanying docker ps are successful
	out, err := s.d.Cmd("run", "-d", "busybox", "top")
	c.Assert(err, check.IsNil)

	id := strings.TrimSpace(out)

	out, err = s.d.Cmd("ps")
	c.Assert(err, check.IsNil)
	c.Assert(assertContainerList(out, []string{id}), check.Equals, true)
}

func (s *DockerAuthzV2Suite) TestAuthZPluginRejectVolumeRequests(c *check.C) {
	testRequires(c, IsAmd64)
	// Install authz plugin
	_, err := s.d.Cmd("plugin", "install", "--grant-all-permissions", authzPluginNameWithTag)
	c.Assert(err, checker.IsNil)

	// restart the daemon with the plugin
	c.Assert(s.d.Restart("--authorization-plugin="+authzPluginNameWithTag), check.IsNil)

	// defer disabling the plugin
	defer func() {
		c.Assert(s.d.Restart(), check.IsNil)
		_, err = s.d.Cmd("plugin", "disable", authzPluginNameWithTag)
		c.Assert(err, checker.IsNil)
		_, err = s.d.Cmd("plugin", "rm", authzPluginNameWithTag)
		c.Assert(err, checker.IsNil)
	}()

	out, err := s.d.Cmd("volume", "create")
	c.Assert(err, check.NotNil)
	c.Assert(out, checker.Contains, fmt.Sprintf("Error response from daemon: plugin %s failed with error:", authzPluginNameWithTag))

	out, err = s.d.Cmd("volume", "ls")
	c.Assert(err, check.NotNil)
	c.Assert(out, checker.Contains, fmt.Sprintf("Error response from daemon: plugin %s failed with error:", authzPluginNameWithTag))

	// The plugin will block the command before it can determine the volume does not exist
	out, err = s.d.Cmd("volume", "rm", "test")
	c.Assert(err, check.NotNil)
	c.Assert(out, checker.Contains, fmt.Sprintf("Error response from daemon: plugin %s failed with error:", authzPluginNameWithTag))

	out, err = s.d.Cmd("volume", "inspect", "test")
	c.Assert(err, check.NotNil)
	c.Assert(out, checker.Contains, fmt.Sprintf("Error response from daemon: plugin %s failed with error:", authzPluginNameWithTag))

	out, err = s.d.Cmd("volume", "prune", "-f")
	c.Assert(err, check.NotNil)
	c.Assert(out, checker.Contains, fmt.Sprintf("Error response from daemon: plugin %s failed with error:", authzPluginNameWithTag))
}

func (s *DockerAuthzV2Suite) TestAuthZPluginBadManifestFailsDaemonStart(c *check.C) {
	// Install authz plugin with bad manifest
	_, err := s.d.Cmd("plugin", "install", "--grant-all-permissions", authzPluginBadManifestName)
	c.Assert(err, checker.IsNil)

	// start the daemon with the plugin, it will error
	c.Assert(s.d.Restart("--authorization-plugin="+authzPluginBadManifestName), check.NotNil)

	// restarting the daemon without requiring the plugin will succeed
	c.Assert(s.d.Restart(), check.IsNil)
}

func (s *DockerAuthzV2Suite) TestNonexistentAuthZPluginFailsDaemonStart(c *check.C) {
	// start the daemon with a non-existent authz plugin, it will error
	c.Assert(s.d.Restart("--authorization-plugin="+nonexistentAuthzPluginName), check.NotNil)

	// restarting the daemon without requiring the plugin will succeed
	c.Assert(s.d.Restart(), check.IsNil)
}
