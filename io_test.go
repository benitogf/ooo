package ooo_test

import (
	"os"
	"testing"

	"github.com/benitogf/ooo"
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

func TestIObasic(t *testing.T) {
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	err := ooo.Set(server, THING1_PATH, Thing{
		This: "this",
		That: "that",
	})
	require.NoError(t, err)
	err = ooo.Set(server, THING2_PATH, Thing{
		This: "here",
		That: "there",
	})
	require.NoError(t, err)

	thing1, err := ooo.Get[Thing](server, THING1_PATH)
	require.NoError(t, err)

	require.Equal(t, "this", thing1.Data.This)
	require.Equal(t, "that", thing1.Data.That)

	thing2, err := ooo.Get[Thing](server, THING2_PATH)
	require.NoError(t, err)

	require.Equal(t, "here", thing2.Data.This)
	require.Equal(t, "there", thing2.Data.That)

	err = ooo.Push(server, THINGS_PATH, thing1)
	require.NoError(t, err)
	err = ooo.Push(server, THINGS_PATH, thing2)
	require.NoError(t, err)

	things, err := ooo.GetList[Thing](server, THINGS_PATH)
	require.NoError(t, err)
	require.Equal(t, 2, len(things))

	err = ooo.Set(server, string(THINGS_BASE_PATH)+"/what", Thing{
		This: "what",
		That: "how",
	})
	require.NoError(t, err)

	things, err = ooo.GetList[Thing](server, THINGS_PATH)
	require.NoError(t, err)
	require.Equal(t, 3, len(things))
	require.Equal(t, "what", things[2].Data.This)
}

func TestRemoteIO(t *testing.T) {
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	err := ooo.RemoteSet(server.Client, false, server.Address, THING1_PATH, Thing{
		This: "this",
		That: "that",
	})
	require.NoError(t, err)
	err = ooo.RemoteSet(server.Client, false, server.Address, THING2_PATH, Thing{
		This: "here",
		That: "there",
	})
	require.NoError(t, err)

	thing1, err := ooo.RemoteGet[Thing](server.Client, false, server.Address, THING1_PATH)
	require.NoError(t, err)

	require.Equal(t, "this", thing1.Data.This)
	require.Equal(t, "that", thing1.Data.That)

	thing2, err := ooo.RemoteGet[Thing](server.Client, false, server.Address, THING2_PATH)
	require.NoError(t, err)

	require.Equal(t, "here", thing2.Data.This)
	require.Equal(t, "there", thing2.Data.That)

	err = ooo.RemotePush(server.Client, false, server.Address, THINGS_PATH, thing1)
	require.NoError(t, err)
	err = ooo.RemotePush(server.Client, false, server.Address, THINGS_PATH, thing2)
	require.NoError(t, err)

	things, err := ooo.RemoteGetList[Thing](server.Client, false, server.Address, THINGS_PATH)
	require.NoError(t, err)
	require.Equal(t, 2, len(things))

	err = ooo.RemoteSet(server.Client, false, server.Address, string(THINGS_BASE_PATH)+"/what", Thing{
		This: "what",
		That: "how",
	})
	require.NoError(t, err)

	things, err = ooo.RemoteGetList[Thing](server.Client, false, server.Address, THINGS_PATH)
	require.NoError(t, err)
	require.Equal(t, 3, len(things))
	require.Equal(t, "what", things[2].Data.This)
}
