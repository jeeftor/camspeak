package frigate

import (
	"path/filepath"
	"testing"

	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/db"
)

func TestRediscoveryPreservesCameraPreferences(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "camspeak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cam := config.CameraConfig{
		Type: "hikvision", IP: "old", User: "old", Gain: 0,
		SortOrder: 7, VisionPrompt: "keep", AirPlayName: "Front", AirPlayModel: "model", Enabled: false,
	}
	if err := config.SaveCamera(database, "front", cam); err != nil {
		t.Fatal(err)
	}
	if err := SaveToDB(database, []DiscoveredCamera{{Name: "front", Type: "hikvision", IP: "new", User: "new", Channel: 2}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(database)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.Cameras["front"]
	if got.IP != "new" || got.User != "new" || got.Channel != 2 {
		t.Fatal("rediscovery failed to apply new connection details")
	}
	if got.Gain != 0 || got.SortOrder != 7 || got.VisionPrompt != "keep" {
		t.Fatal("rediscovery lost playback or display preferences")
	}
	if got.AirPlayName != "Front" || got.Enabled {
		t.Fatal("rediscovery lost AirPlay or enabled preferences")
	}
}
