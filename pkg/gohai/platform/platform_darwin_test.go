// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestUpdateArchInfo(t *testing.T) {
	uname := &unix.Utsname{}
	sysname := "A"
	copy(uname.Sysname[:], []byte(sysname))
	nodename := "B"
	copy(uname.Nodename[:], []byte(nodename))
	release := "C"
	copy(uname.Release[:], []byte(release))
	version := "D"
	copy(uname.Version[:], []byte(version))
	machine := "E"
	copy(uname.Machine[:], []byte(machine))

	expected := map[string]string{
		"kernel_name":    sysname,
		"hostname":       nodename,
		"kernel_release": release,
		"machine":        machine,
		"processor":      getUnameProcessor(),
		"os":             sysname,
		"kernel_version": version,
	}

	archInfo := map[string]string{}
	updateArchInfo(archInfo, uname)

	require.Equal(t, expected, archInfo)
}
