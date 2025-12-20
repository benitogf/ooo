package merge_test

import (
	"encoding/json"
	"testing"

	"github.com/benitogf/ooo/merge"
	"github.com/stretchr/testify/require"
)

type NestTwo struct {
	Test string `json:"test"`
}

type NestOne struct {
	Test string  `json:"test"`
	Nest NestTwo `json:"nest"`
}

type TestOne struct {
	Test    string  `json:"test"`
	Another string  `json:"another"`
	Nest    NestOne `json:"nest"`
}

func TestMerge(t *testing.T) {
	testLeft := json.RawMessage(`{"test":"123"}`)
	testRight := json.RawMessage(`{"test":"1234"}`)
	merged, info, err := merge.MergeBytes(testLeft, testRight)
	require.NoError(t, err)
	_ = info

	testOne := TestOne{}
	json.Unmarshal(merged, &testOne)
	require.Equal(t, "1234", testOne.Test)
	require.NotZero(t, len(info.Replaced))
}

func TestMergeNoop(t *testing.T) {
	testLeft := json.RawMessage(`{"test":"123"}`)
	testRight := json.RawMessage(`{"test":"123"}`)
	merged, info, err := merge.MergeBytes(testLeft, testRight)
	require.NoError(t, err)
	_ = info

	testOne := TestOne{}
	json.Unmarshal(merged, &testOne)
	require.Equal(t, "123", testOne.Test)
	require.Zero(t, len(info.Replaced))
}

func TestMergeNested(t *testing.T) {
	testLeft := json.RawMessage(`{"test":"123", "nest":{"test": "no"}}`)
	testRight := json.RawMessage(`{"test":"1234"}`)
	merged, info, err := merge.MergeBytes(testLeft, testRight)
	require.NoError(t, err)
	_ = info

	testOne := TestOne{}
	json.Unmarshal(merged, &testOne)
	require.Equal(t, "no", testOne.Nest.Test)
}

func TestMergeNested2(t *testing.T) {
	testLeft := json.RawMessage(`{"test":"123", "nest":{"test": "no", "nest":{"test": "no"}}}`)
	testRight := json.RawMessage(`{"test":"1234"}`)
	merged, info, err := merge.MergeBytes(testLeft, testRight)
	require.NoError(t, err)
	_ = info

	testOne := TestOne{}
	json.Unmarshal(merged, &testOne)
	require.Equal(t, "no", testOne.Nest.Nest.Test)
}

func TestNewKey(t *testing.T) {
	testLeft := json.RawMessage(`{"test":"123"}`)
	testRight := json.RawMessage(`{"test":"1234", "another":"no"}`)
	merged, info, err := merge.MergeBytes(testLeft, testRight)
	require.NoError(t, err)
	_ = info

	testOne := TestOne{}
	json.Unmarshal(merged, &testOne)
	require.Equal(t, "1234", testOne.Test)
	require.Equal(t, "no", testOne.Another)
	require.Equal(t, 2, len(info.Replaced))
}

func BenchmarkMergeBytes(b *testing.B) {
	testLeft := json.RawMessage(`{"test":"123","count":42,"active":true,"nested":{"a":"b","c":"d"}}`)
	testRight := json.RawMessage(`{"test":"1234","count":43}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = merge.MergeBytes(testLeft, testRight)
	}
}
