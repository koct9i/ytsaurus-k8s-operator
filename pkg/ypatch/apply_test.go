package ypatch

import (
	"context"
	"reflect"
	"testing"

	"go.ytsaurus.tech/yt/go/ypath"
	"go.ytsaurus.tech/yt/go/yt"
	"go.ytsaurus.tech/yt/go/yterrors"
)

type mockCypressClient struct {
	nodes map[string]any
	ops   []string

	failNextSetWithResolveError bool
}

func newMockCypressClient(nodes map[string]any) *mockCypressClient {
	return &mockCypressClient{nodes: nodes}
}

func (m *mockCypressClient) CreateNode(_ context.Context, path ypath.YPath, typ yt.NodeType, options *yt.CreateNodeOptions) (yt.NodeID, error) {
	m.ops = append(m.ops, "create "+string(path.YPath()))
	if typ != yt.NodeMap {
		return yt.NodeID{}, nil
	}
	attrs := map[string]any{}
	if options != nil {
		for k, v := range options.Attributes {
			attrs[k] = v
		}
	}
	m.nodes[string(path.YPath())] = attrs
	return yt.NodeID{}, nil
}

func (m *mockCypressClient) CreateObject(context.Context, yt.NodeType, *yt.CreateObjectOptions) (yt.NodeID, error) {
	panic("not implemented")
}

func (m *mockCypressClient) NodeExists(_ context.Context, path ypath.YPath, _ *yt.NodeExistsOptions) (bool, error) {
	_, ok := m.nodes[string(path.YPath())]
	return ok, nil
}

func (m *mockCypressClient) RemoveNode(_ context.Context, path ypath.YPath, _ *yt.RemoveNodeOptions) error {
	m.ops = append(m.ops, "remove "+string(path.YPath()))
	delete(m.nodes, string(path.YPath()))
	return nil
}

func (m *mockCypressClient) GetNode(_ context.Context, path ypath.YPath, result any, _ *yt.GetNodeOptions) error {
	m.ops = append(m.ops, "get "+string(path.YPath()))
	value, ok := m.nodes[string(path.YPath())]
	if !ok {
		return &yterrors.Error{Code: yterrors.CodeResolveError, Message: "missing node"}
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(value))
	return nil
}

func (m *mockCypressClient) SetNode(_ context.Context, path ypath.YPath, value any, _ *yt.SetNodeOptions) error {
	m.ops = append(m.ops, "set "+string(path.YPath()))
	if m.failNextSetWithResolveError {
		m.failNextSetWithResolveError = false
		return &yterrors.Error{Code: yterrors.CodeResolveError, Message: "missing parent attribute"}
	}
	m.nodes[string(path.YPath())] = value
	return nil
}

func (m *mockCypressClient) MultisetAttributes(context.Context, ypath.YPath, map[string]any, *yt.MultisetAttributesOptions) error {
	panic("not implemented")
}
func (m *mockCypressClient) ListNode(context.Context, ypath.YPath, any, *yt.ListNodeOptions) error {
	panic("not implemented")
}
func (m *mockCypressClient) CopyNode(context.Context, ypath.YPath, ypath.YPath, *yt.CopyNodeOptions) (yt.NodeID, error) {
	panic("not implemented")
}
func (m *mockCypressClient) MoveNode(context.Context, ypath.YPath, ypath.YPath, *yt.MoveNodeOptions) (yt.NodeID, error) {
	panic("not implemented")
}
func (m *mockCypressClient) LinkNode(context.Context, ypath.YPath, ypath.YPath, *yt.LinkNodeOptions) (yt.NodeID, error) {
	panic("not implemented")
}

func TestCypressPatchTargetApplyPatchCommonCases(t *testing.T) {
	ctx := context.Background()
	client := newMockCypressClient(map[string]any{
		"//source": "copied-value",
		"//move":   "moved-value",
		"//check":  map[string]any{"ok": true},
		"//remove": "old-value",
	})
	target := CypressPatchTarget{Client: client}

	patch := Patch{
		Add("//added", "new-value"),
		Replace("//replaced", map[string]any{"answer": 42}),
		Copy("//copied", "//source"),
		Move("//moved", "//move"),
		Remove("//remove"),
		Test("//check", map[string]any{"ok": true}),
	}

	if err := target.ApplyPatch(ctx, "", patch); err != nil {
		t.Fatalf("ApplyPatch() failed: %v", err)
	}

	assertNodeEqual(t, client.nodes, "//added", "new-value")
	assertNodeEqual(t, client.nodes, "//replaced", map[string]any{"answer": 42})
	assertNodeEqual(t, client.nodes, "//copied", "copied-value")
	assertNodeEqual(t, client.nodes, "//moved", "moved-value")
	if _, ok := client.nodes["//move"]; ok {
		t.Fatalf("source of move was not removed")
	}
	if _, ok := client.nodes["//remove"]; ok {
		t.Fatalf("removed node still exists")
	}
}

func TestCypressPatchTargetApplyPatchSetUsesSortedPaths(t *testing.T) {
	ctx := context.Background()
	client := newMockCypressClient(map[string]any{})
	target := CypressPatchTarget{Client: client}

	patchSet := PatchSet{
		"//b": {Replace("/value", 2)},
		"//a": {Replace("/value", 1)},
	}

	if err := target.ApplyPatchSet(ctx, "", patchSet); err != nil {
		t.Fatalf("ApplyPatchSet() failed: %v", err)
	}

	wantOps := []string{"set //a/value", "set //b/value"}
	if !reflect.DeepEqual(client.ops, wantOps) {
		t.Fatalf("ops = %#v, want %#v", client.ops, wantOps)
	}
}

func TestCypressPatchTargetSetNodeCreatesNodeWhenParentAttributeIsMissing(t *testing.T) {
	ctx := context.Background()
	client := newMockCypressClient(map[string]any{})
	client.failNextSetWithResolveError = true
	target := CypressPatchTarget{Client: client}

	if err := target.ApplyPatch(ctx, "", Patch{Replace("//sys/accounts/foo/@resource_limits", map[string]any{"node_count": 10})}); err != nil {
		t.Fatalf("ApplyPatch() failed: %v", err)
	}

	assertNodeEqual(t, client.nodes, "//sys/accounts/foo", map[string]any{
		"resource_limits": map[string]any{"node_count": 10},
	})
	wantOps := []string{"set //sys/accounts/foo/@resource_limits", "create //sys/accounts/foo"}
	if !reflect.DeepEqual(client.ops, wantOps) {
		t.Fatalf("ops = %#v, want %#v", client.ops, wantOps)
	}
}

func assertNodeEqual(t *testing.T, nodes map[string]any, path string, want any) {
	t.Helper()
	got, ok := nodes[path]
	if !ok {
		t.Fatalf("node %s does not exist", path)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("node %s = %#v, want %#v", path, got, want)
	}
}
