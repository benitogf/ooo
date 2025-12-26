package ooo

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoubleShutdown(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	server.Close(os.Interrupt)
}

func TestDoubleStart(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:9889")
	server.Start("localhost:9889")
	defer server.Close(os.Interrupt)
}

func TestRestart(t *testing.T) {
	t.Skip()
	server := Server{}
	server.Silence = true
	server.Start("localhost:9889")
	server.Close(os.Interrupt)
	// https://golang.org/pkg/net/http/#example_Server_Shutdown
	server.Start("localhost:9889")
	defer server.Close(os.Interrupt)
}

// TODO: find a way to test this
// func TestDeadline(t *testing.T) {
// 	if runtime.GOOS == "windows" {
// 		// TODO: investigate how to simulate a delay in the request on windows
// 		t.Skip()
// 	}
// 	app := Server{
// 		Deadline: 1 * time.Nanosecond,
// 		Silence:  true,
// 	}
// 	app.Start("localhost:0")
// 	defer app.Close(os.Interrupt)

// 	var jsonStr = []byte(`{"data":"test"}`)
// 	req := httptest.NewRequest("POST", "/test", bytes.NewBuffer(jsonStr))
// 	w := httptest.NewRecorder()
// 	app.Router.ServeHTTP(w, req)
// 	resp := w.Result()
// 	require.Equal(t, 503, resp.StatusCode)
// }

func TestServerValidate(t *testing.T) {
	// Valid config
	server := &Server{}
	require.NoError(t, server.Validate())

	// ForcePatch and NoPatch both enabled
	server = &Server{
		ForcePatch: true,
		NoPatch:    true,
	}
	err := server.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ForcePatch and NoPatch cannot both be enabled")

	// Negative Workers
	server = &Server{
		Workers: -1,
	}
	err = server.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Workers cannot be negative")

	// Negative Deadline
	server = &Server{
		Deadline: -1,
	}
	err = server.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Deadline cannot be negative")
}
