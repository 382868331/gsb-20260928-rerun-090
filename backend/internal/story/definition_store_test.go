package story

import "testing"

func TestFileDefinitionStoreLoadsLatestVersionLast(t *testing.T) {
	store, err := NewFileDefinitionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v1 := testStory()
	v2 := testStory()
	v2.Version = "v2"
	n := v2.Nodes["start"]
	n.Choices[0].Text = "changed"
	v2.Nodes["start"] = n
	if err := store.SaveDefinition(v1); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDefinition(v2); err != nil {
		t.Fatal(err)
	}
	defs, err := store.ListDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	catalog := NewVersionedCatalog(defs...)
	latest, err := catalog.Get("test")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != "v2" {
		t.Fatalf("latest pointer must win after restart load, got %s", latest.Version)
	}
}
