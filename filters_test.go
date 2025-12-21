package ooo

import (
	"bytes"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/benitogf/jsondiff"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"

	"github.com/stretchr/testify/require"
)

func TestFilters(t *testing.T) {
	app := Server{}
	app.Silence = true
	unacceptedData := json.RawMessage(`{"test":false}`)
	acceptedData := json.RawMessage(`{"test":true}`)
	uninterceptedData := json.RawMessage(`{"intercepted":false}`)
	interceptedData := json.RawMessage(`{"intercepted":true}`)
	filteredData := json.RawMessage(`{"filtered":true}`)
	unfilteredData := json.RawMessage(`{"filtered":false}`)
	notified := false
	app.WriteFilter("test1", func(key string, data json.RawMessage) (json.RawMessage, error) {
		comparison, _ := jsondiff.Compare(data, unfilteredData, &jsondiff.Options{})
		if comparison != jsondiff.FullMatch {
			return nil, errors.New("filtered")
		}

		return data, nil
	})
	app.WriteFilter("test/*", func(key string, data json.RawMessage) (json.RawMessage, error) {
		comparison, _ := jsondiff.Compare(data, acceptedData, &jsondiff.Options{})
		if comparison != jsondiff.FullMatch {
			return nil, errors.New("filtered")
		}

		return data, nil
	})
	// Use meta-based list filter for bag/*
	app.ReadListFilter("bag/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
		return []meta.Object{{Data: interceptedData}}, nil
	})
	// Use meta-based object filter for bag/1 (single item read)
	app.ReadObjectFilter("bag/*", func(key string, obj meta.Object) (meta.Object, error) {
		return meta.Object{Data: interceptedData, Created: 1, Index: "1"}, nil
	})

	app.WriteFilter("book/*", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return data, nil
	})
	// Use meta-based list filter for book/*
	app.ReadListFilter("book/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
		return objs, nil
	})
	app.AfterWriteFilter("flyer", func(key string) {
		notified = true
	})

	app.Start("localhost:9889")
	defer app.Close(os.Interrupt)
	_, err := app.filters.Write.Check("test/1", unacceptedData, false)
	require.Error(t, err)
	_, err = app.filters.Write.Check("test/1", acceptedData, false)
	require.NoError(t, err)
	// Test meta-based object filter
	obj, err := app.filters.ReadObject.Check("bag/1", meta.Object{Data: uninterceptedData}, false)
	require.NoError(t, err)
	comparison, _ := jsondiff.Compare(obj.Data, interceptedData, &jsondiff.Options{})
	require.Equal(t, comparison, jsondiff.FullMatch)
	_, err = app.filters.Write.Check("test1", filteredData, false)
	require.Error(t, err)
	_, err = app.filters.Write.Check("test1", unfilteredData, false)
	require.NoError(t, err)
	// test static - write filters
	_, err = app.filters.Write.Check("book", unacceptedData, true)
	require.Error(t, err)
	_, err = app.filters.Write.Check("book/1/1", unacceptedData, true)
	require.Error(t, err)
	_, err = app.filters.Write.Check("book/1/1/1", unacceptedData, true)
	require.Error(t, err)
	// test static - read list filters
	_, err = app.filters.ReadList.Check("book", nil, true)
	require.Error(t, err)
	_, err = app.filters.ReadList.Check("book/1/1", nil, true)
	require.Error(t, err)
	_, err = app.filters.ReadList.Check("book/1/1/1", nil, true)
	require.Error(t, err)
	_, err = app.filters.Write.Check("book/1", unfilteredData, true)
	require.NoError(t, err)
	_, err = app.filters.ReadList.Check("book/1", nil, true)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/test/1", bytes.NewBuffer(TEST_DATA))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 400, resp.StatusCode)

	req = httptest.NewRequest("POST", "/bag/1", bytes.NewBuffer(uninterceptedData))
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode)

	req = httptest.NewRequest("POST", "/flyer", bytes.NewBuffer(TEST_DATA))
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode)
	require.True(t, notified)

	req = httptest.NewRequest("GET", "/bag/1", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	// Response is a meta.Object, decode and check the data field
	var respObj meta.Object
	err = json.Unmarshal(body, &respObj)
	require.NoError(t, err)
	comparison, _ = jsondiff.Compare(respObj.Data, interceptedData, &jsondiff.Options{})
	require.Equal(t, comparison, jsondiff.FullMatch)
}

func TestServerValidate(t *testing.T) {
	// Valid config
	app := &Server{}
	require.NoError(t, app.Validate())

	// ForcePatch and NoPatch both enabled
	app = &Server{
		ForcePatch: true,
		NoPatch:    true,
	}
	err := app.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ForcePatch and NoPatch cannot both be enabled")

	// Negative Workers
	app = &Server{
		Workers: -1,
	}
	err = app.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Workers cannot be negative")

	// Negative Deadline
	app = &Server{
		Deadline: -1,
	}
	err = app.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Deadline cannot be negative")
}
