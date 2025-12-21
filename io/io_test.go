package io_test

import (
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/io"
	"github.com/stretchr/testify/require"
)

type Thing struct {
	This string `json:"this"`
	That string `json:"that"`
}

const THING1_PATH = "thing1"
const THING2_PATH = "thing2"
const THINGS_BASE_PATH = "things"
const THINGS_PATH = THINGS_BASE_PATH + "/*"

const THING_INVALID_PATH = "invalid/thing"
const THINGS_INVALID_PATH = "invalid/things/*"

func TestIObasic(t *testing.T) {
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	err := io.Set(server, THING1_PATH, Thing{
		This: "this",
		That: "that",
	})
	require.NoError(t, err)
	err = io.Set(server, THING2_PATH, Thing{
		This: "here",
		That: "there",
	})
	require.NoError(t, err)

	thing1, err := io.Get[Thing](server, THING1_PATH)
	require.NoError(t, err)

	require.Equal(t, "this", thing1.Data.This)
	require.Equal(t, "that", thing1.Data.That)

	thing2, err := io.Get[Thing](server, THING2_PATH)
	require.NoError(t, err)

	require.Equal(t, "here", thing2.Data.This)
	require.Equal(t, "there", thing2.Data.That)

	index, err := io.Push(server, THINGS_PATH, thing1.Data)
	require.NoError(t, err)
	require.NotEmpty(t, index)
	if runtime.GOOS == "windows" {
		time.Sleep(10 * time.Millisecond)
	}
	index, err = io.Push(server, THINGS_PATH, thing2.Data)
	require.NoError(t, err)
	require.NotEmpty(t, index)
	if runtime.GOOS == "windows" {
		time.Sleep(10 * time.Millisecond)
	}

	things, err := io.GetList[Thing](server, THINGS_PATH)
	require.NoError(t, err)
	require.Equal(t, 2, len(things))
	require.Equal(t, "this", things[0].Data.This)
	require.Equal(t, "here", things[1].Data.This)

	err = io.Set(server, string(THINGS_BASE_PATH)+"/what", Thing{
		This: "what",
		That: "how",
	})
	require.NoError(t, err)

	things, err = io.GetList[Thing](server, THINGS_PATH)
	require.NoError(t, err)
	require.Equal(t, 3, len(things))
	require.Equal(t, "what", things[2].Data.This)
}

func TestRemoteIO(t *testing.T) {
	server := &ooo.Server{}
	server.Silence = true
	server.Static = true
	server.OpenFilter(THING1_PATH)
	server.OpenFilter(THING2_PATH)
	server.OpenFilter(THINGS_PATH)
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	cfg := io.RemoteConfig{
		Client: server.Client,
		Host:   server.Address,
	}

	err := io.RemoteSet(cfg, THING_INVALID_PATH, Thing{
		This: "this",
		That: "that",
	})
	require.Error(t, err)

	_, err = io.RemoteGet[Thing](cfg, THING_INVALID_PATH)
	require.Error(t, err)

	_, err = io.RemoteGetList[Thing](cfg, THINGS_INVALID_PATH)
	require.Error(t, err)

	err = io.RemoteSet(cfg, THING1_PATH, Thing{
		This: "this",
		That: "that",
	})
	require.NoError(t, err)
	err = io.RemoteSet(cfg, THING2_PATH, Thing{
		This: "here",
		That: "there",
	})
	require.NoError(t, err)

	thing1, err := io.RemoteGet[Thing](cfg, THING1_PATH)
	require.NoError(t, err)

	require.Equal(t, "this", thing1.Data.This)
	require.Equal(t, "that", thing1.Data.That)

	thing2, err := io.RemoteGet[Thing](cfg, THING2_PATH)
	require.NoError(t, err)

	require.Equal(t, "here", thing2.Data.This)
	require.Equal(t, "there", thing2.Data.That)

	err = io.RemotePush(cfg, THINGS_PATH, thing1.Data)
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		time.Sleep(10 * time.Millisecond)
	}
	err = io.RemotePush(cfg, THINGS_PATH, thing2.Data)
	require.NoError(t, err)

	err = io.RemotePush(cfg, THINGS_INVALID_PATH, thing1.Data)
	require.Error(t, err)

	things, err := io.RemoteGetList[Thing](cfg, THINGS_PATH)
	require.NoError(t, err)
	require.Equal(t, 2, len(things))
	require.Equal(t, "this", things[0].Data.This)
	require.Equal(t, "here", things[1].Data.This)

	err = io.RemoteSet(cfg, string(THINGS_BASE_PATH)+"/what", Thing{
		This: "what",
		That: "how",
	})
	require.NoError(t, err)

	things, err = io.RemoteGetList[Thing](cfg, THINGS_PATH)
	require.NoError(t, err)
	require.Equal(t, 3, len(things))
	require.Equal(t, "this", things[0].Data.This)
	require.Equal(t, "here", things[1].Data.This)
	require.Equal(t, "what", things[2].Data.This)
}

func TestRemoteDelete(t *testing.T) {
	server := &ooo.Server{}
	server.Silence = true
	server.Static = true
	server.OpenFilter(THING1_PATH)
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	cfg := io.RemoteConfig{
		Client: server.Client,
		Host:   server.Address,
	}

	// Set an item
	err := io.RemoteSet(cfg, THING1_PATH, Thing{
		This: "to-delete",
		That: "soon",
	})
	require.NoError(t, err)

	// Verify it exists
	thing, err := io.RemoteGet[Thing](cfg, THING1_PATH)
	require.NoError(t, err)
	require.Equal(t, "to-delete", thing.Data.This)

	// Delete it
	err = io.RemoteDelete(cfg, THING1_PATH)
	require.NoError(t, err)

	// Verify it returns ErrEmptyKey after deletion
	_, err = io.RemoteGet[Thing](cfg, THING1_PATH)
	require.ErrorIs(t, err, io.ErrEmptyKey)
}

func TestRemoteEmptyKeyVsRouteNotDefined(t *testing.T) {
	server := &ooo.Server{}
	server.Silence = true
	server.Static = true
	server.OpenFilter("valid/*")
	server.OpenFilter("validitem")
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	cfg := io.RemoteConfig{
		Client: server.Client,
		Host:   server.Address,
	}

	// Test 1: Route not defined (400 Bad Request) - path has no filter registered
	_, err := io.RemoteGet[Thing](cfg, "undefined")
	require.Error(t, err)
	require.ErrorIs(t, err, io.ErrRequestFailed)

	// Test 2: Empty key (404 Not Found) - path has filter but no data
	_, err = io.RemoteGet[Thing](cfg, "validitem")
	require.Error(t, err)
	require.ErrorIs(t, err, io.ErrEmptyKey)

	// Test 3: After setting data, should succeed
	err = io.RemoteSet(cfg, "validitem", Thing{This: "test", That: "data"})
	require.NoError(t, err)

	thing, err := io.RemoteGet[Thing](cfg, "validitem")
	require.NoError(t, err)
	require.Equal(t, "test", thing.Data.This)

	// Test 4: After deleting, should return ErrEmptyKey again
	err = io.RemoteDelete(cfg, "validitem")
	require.NoError(t, err)

	_, err = io.RemoteGet[Thing](cfg, "validitem")
	require.Error(t, err)
	require.ErrorIs(t, err, io.ErrEmptyKey)

	// Test 5: List path - route not defined
	_, err = io.RemoteGetList[Thing](cfg, "undefined/*")
	require.Error(t, err)
	require.ErrorIs(t, err, io.ErrRequestFailed)

	// Test 6: List path - empty list (valid route, no data) returns empty slice
	things, err := io.RemoteGetList[Thing](cfg, "valid/*")
	require.NoError(t, err)
	require.Empty(t, things)
}

func TestRemoteConfigValidation(t *testing.T) {
	// Missing Client
	cfg := io.RemoteConfig{
		Host: "localhost:8080",
	}
	err := io.RemoteSet(cfg, "test", Thing{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Client is required")

	// Missing Host
	cfg = io.RemoteConfig{
		Client: &http.Client{},
	}
	err = io.RemoteSet(cfg, "test", Thing{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Host is required")
}

func TestRemotePathValidation(t *testing.T) {
	cfg := io.RemoteConfig{
		Client: &http.Client{},
		Host:   "localhost:8080",
	}

	// RemoteSet with list path should fail
	err := io.RemoteSet(cfg, "things/*", Thing{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "glob not allowed")

	// RemotePush with non-list path should fail
	err = io.RemotePush(cfg, "thing1", Thing{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "glob required")

	// RemoteGet with list path should fail
	_, err = io.RemoteGet[Thing](cfg, "things/*")
	require.Error(t, err)
	require.Contains(t, err.Error(), "glob not allowed")

	// RemoteGetList with non-list path should fail
	_, err = io.RemoteGetList[Thing](cfg, "thing1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "glob required")
}

func TestLocalPathValidation(t *testing.T) {
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Set with list path should fail
	err := io.Set(server, "things/*", Thing{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "glob not allowed")

	// Push with non-list path should fail
	_, err = io.Push(server, "thing1", Thing{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "glob pattern required")

	// Get with list path should fail
	_, err = io.Get[Thing](server, "things/*")
	require.Error(t, err)
	require.Contains(t, err.Error(), "glob not allowed")

	// GetList with non-list path should fail
	_, err = io.GetList[Thing](server, "thing1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "glob required")
}
