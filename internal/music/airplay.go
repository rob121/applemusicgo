package music

import (
	"encoding/json"
	"fmt"
	"strings"
)

const listAirPlayJXA = `
try { var itunes = Application("Music"); } catch(e) { var itunes = Application("iTunes"); }
var airPlayDevices = itunes.airplayDevices();
var results = [];
for (var i = 0; i < airPlayDevices.length; i++) {
  var d = airPlayDevices[i];
  var deviceData = {};
  if (d.networkAddress()) {
    deviceData.id = d.networkAddress().replace(/:/g, "-");
  } else {
    deviceData.id = d.name();
  }
  deviceData.name = d.name();
  deviceData.kind = d.kind();
  deviceData.active = d.active();
  deviceData.selected = d.selected();
  deviceData.sound_volume = d.soundVolume();
  deviceData.supports_video = d.supportsVideo();
  deviceData.supports_audio = d.supportsAudio();
  deviceData.network_address = d.networkAddress();
  results.push(deviceData);
}
JSON.stringify(results);
`

type AirPlayDevice map[string]any

func ListAirPlayDevices() ([]AirPlayDevice, error) {
	out, err := RunJXA(listAirPlayJXA)
	if err != nil {
		return nil, err
	}
	var devices []AirPlayDevice
	if err := json.Unmarshal([]byte(out), &devices); err != nil {
		return nil, err
	}
	return devices, nil
}

func FindAirPlayDevice(devices []AirPlayDevice, id string) (AirPlayDevice, bool) {
	for _, d := range devices {
		if d["id"] == id {
			return d, true
		}
	}
	return nil, false
}

func SetAirPlaySelection(id string, selected bool) error {
	sel := "true"
	if !selected {
		sel = "false"
	}
	script := fmt.Sprintf(`
try { var itunes = Application("Music"); } catch(e) { var itunes = Application("iTunes"); }
var cleanId = %q.replace(/-/g, ":");
var devices = itunes.airplayDevices();
for (var i = 0; i < devices.length; i++) {
  var d = devices[i];
  if (d.networkAddress() === cleanId || d.name() === %q) {
    d.selected = %s;
    "ok";
  }
}
"fail";
`, id, id, sel)
	out, err := RunJXA(script)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "ok" {
		return fmt.Errorf("airplay device not found")
	}
	return nil
}

func SetAirPlayVolume(id string, level int) error {
	script := fmt.Sprintf(`
try { var itunes = Application("Music"); } catch(e) { var itunes = Application("iTunes"); }
var cleanId = %q.replace(/-/g, ":");
var devices = itunes.airplayDevices();
for (var i = 0; i < devices.length; i++) {
  var d = devices[i];
  if (d.networkAddress() === cleanId || d.name() === %q) {
    d.soundVolume = %d;
    "ok";
  }
}
"fail";
`, id, id, level)
	out, err := RunJXA(script)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "ok" {
		return fmt.Errorf("airplay device not found")
	}
	return nil
}
