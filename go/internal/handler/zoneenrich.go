package handler

import (
	"fmt"
	"math"
	"strconv"

	"github.com/olizimmermann/drohnenwetter/internal/api"
)

// Keys added to DiPUL area maps by enrichZones. Prefixed with "_" so they
// never clash with DiPUL's own fields.
const (
	keyLinkDE       = "_linkDE"
	keyLinkEN       = "_linkEN"
	keyCTRMaxAGL    = "_ctrMaxAGL"    // float64, metres above ground
	keyGroundMSL    = "_groundMSL"    // float64, metres above sea level
	keyAerodromeMSL = "_aerodromeMSL" // float64, metres above sea level
)

// ctrCeilingAboveAerodrome is the general-clearance ceiling of UAS zones 2–4
// relative to aerodrome elevation (NfL 2026-1-3960). DiPUL sets the zone
// floor of these zones to exactly aerodrome elevation + this value (AMSL).
var ctrCeilingAboveAerodrome = map[int]float64{2: 25, 3: 45, 4: 100}

// ctrMaxAGL computes the general-clearance height limit above ground at the
// take-off point. To stay on the safe side it assumes the ground is rmse
// metres higher than measured and rounds down.
//
//	Zone 2/3: N m above aerodrome elevation or N m above ground, the lower.
//	Zone 4:   100 m above aerodrome elevation or 50 m above ground, the
//	          higher; 100 m above ground if the take-off point is below
//	          aerodrome elevation.
func ctrMaxAGL(zone int, floorAMSL, groundMSL, rmse float64) (maxAGL, aerodromeMSL float64, ok bool) {
	n, ok := ctrCeilingAboveAerodrome[zone]
	if !ok {
		return 0, 0, false
	}
	aerodromeMSL = floorAMSL - n
	g := groundMSL + rmse
	switch zone {
	case 4:
		if g < aerodromeMSL {
			maxAGL = 100
		} else {
			maxAGL = math.Max(floorAMSL-g, 50)
		}
	default:
		maxAGL = math.Max(math.Min(n, floorAMSL-g), 0)
	}
	return math.Floor(maxAGL), aerodromeMSL, true
}

// enrichZones attaches ED-318 information to the zone-API areas: the DiPUL
// legal-basis link for every area and, for DFS control zones, the height
// limit at this location. Must run before the result is cached (the maps
// are shared afterwards).
func enrichZones(zones []api.AffectedArea, edZones []api.UASZone, ground *api.Elevation) {
	byID := make(map[string]api.UASZone, len(edZones))
	byName := make(map[string]api.UASZone, len(edZones))
	for _, z := range edZones {
		byID[z.ID] = z
		byName[z.Name] = z
	}

	for _, zone := range zones {
		for _, area := range zone.Areas {
			ed, found := byID[fmt.Sprint(area["externalReference"])]
			if !found {
				// Control zones use different IDs in both APIs; match by name.
				ed, found = byName[fmt.Sprint(area["name"])]
			}
			if !found {
				continue
			}
			if ed.LinkDE != "" {
				area[keyLinkDE] = ed.LinkDE
			}
			if ed.LinkEN != "" {
				area[keyLinkEN] = ed.LinkEN
			}

			if ground == nil || ed.LowerRef != "AMSL" || fmt.Sprint(area["legalReference"]) != nflCTR2026 {
				continue
			}
			m := reCTRZone.FindStringSubmatch(fmt.Sprint(area["name"]))
			if m == nil {
				continue
			}
			n, _ := strconv.Atoi(m[1])
			if maxAGL, aerodrome, ok := ctrMaxAGL(n, ed.LowerM, ground.MSL, ground.RMSE); ok {
				area[keyCTRMaxAGL] = maxAGL
				area[keyGroundMSL] = ground.MSL
				area[keyAerodromeMSL] = aerodrome
			}
		}
	}
}
