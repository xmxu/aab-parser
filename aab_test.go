package aab

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xmxu/aab-parser/pb"
)

func TestParseAab(t *testing.T) {
	aab, err := OpenFile("testdata/app.aab")
	if err != nil {
		t.Fatal(err)
	}
	defer aab.Close()

	assert.Equal(t, "com.pworld.club.myapplication", aab.PackageName())
	assert.Equal(t, "1.0", aab.Manifest().VersionName)
	assert.Equal(t, int64(1), aab.Manifest().VersionCode)
	assert.Equal(t, "My Application", aab.Label(nil))
	icon, err := aab.Icon(&pb.Configuration{
		Density: 640,
	})
	if err != nil {
		t.Fatal(err)
	}
	if icon == nil {
		t.Fatal("no icon")
	}
}
