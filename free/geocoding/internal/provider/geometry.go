package provider

import (
	"fmt"
	"math"
)

// ─── Timezone lookup (no external call) ────────────────────────────────────────

// TZAtCoords returns the IANA timezone name for a lat/lng.
// Uses a UTC-offset approximation based on longitude, then refines with a
// small hard-coded table of major metropolitan areas. Suitable for EPG
// scheduling where exact DST boundary precision is not required.
//
// For production-grade precision, embed a full timezone boundary dataset
// (e.g. github.com/evanoberholster/timezoneLookup).
func TZAtCoords(lat, lng float64) string {
	// Major timezone lookup by proximity to known cities (lat, lng, tz)
	type tzPoint struct {
		lat, lng float64
		tz       string
	}
	landmarks := []tzPoint{
		// Americas
		{40.7128, -74.0060, "America/New_York"},
		{41.8781, -87.6298, "America/Chicago"},
		{39.7392, -104.9903, "America/Denver"},
		{34.0522, -118.2437, "America/Los_Angeles"},
		{61.2181, -149.9003, "America/Anchorage"},
		{21.3069, -157.8583, "Pacific/Honolulu"},
		{45.4215, -75.6919, "America/Toronto"},
		{43.6532, -79.3832, "America/Toronto"},
		{45.5017, -73.5673, "America/Toronto"},
		{49.2827, -123.1207, "America/Vancouver"},
		{-23.5505, -46.6333, "America/Sao_Paulo"},
		{-34.6037, -58.3816, "America/Argentina/Buenos_Aires"},
		{19.4326, -99.1332, "America/Mexico_City"},
		// Europe
		{51.5074, -0.1278, "Europe/London"},
		{48.8566, 2.3522, "Europe/Paris"},
		{52.5200, 13.4050, "Europe/Berlin"},
		{55.7558, 37.6173, "Europe/Moscow"},
		{41.0082, 28.9784, "Europe/Istanbul"},
		{59.9139, 10.7522, "Europe/Oslo"},
		{40.4168, -3.7038, "Europe/Madrid"},
		{41.9028, 12.4964, "Europe/Rome"},
		// Asia/Pacific
		{35.6762, 139.6503, "Asia/Tokyo"},
		{37.5665, 126.9780, "Asia/Seoul"},
		{39.9042, 116.4074, "Asia/Shanghai"},
		{22.3193, 114.1694, "Asia/Hong_Kong"},
		{1.3521, 103.8198, "Asia/Singapore"},
		{28.6139, 77.2090, "Asia/Kolkata"},
		{19.0760, 72.8777, "Asia/Kolkata"},
		{31.2304, 121.4737, "Asia/Shanghai"},
		{-33.8688, 151.2093, "Australia/Sydney"},
		{-37.8136, 144.9631, "Australia/Melbourne"},
		// Africa / Middle East
		{30.0444, 31.2357, "Africa/Cairo"},
		{6.5244, 3.3792, "Africa/Lagos"},
		{24.7136, 46.6753, "Asia/Riyadh"},
		{25.2048, 55.2708, "Asia/Dubai"},
	}

	best := "UTC"
	bestDist := math.MaxFloat64
	for _, p := range landmarks {
		dlat := lat - p.lat
		dlng := lng - p.lng
		dist := dlat*dlat + dlng*dlng // squared Euclidean — sufficient for nearest-point
		if dist < bestDist {
			bestDist = dist
			best = p.tz
		}
	}

	// If we're within ~3 degrees of a known landmark, trust it.
	if bestDist <= 9.0 {
		return best
	}

	// Fallback: approximate by UTC offset from longitude.
	offsetHours := math.Round(lng / 15.0)
	sign := "+"
	if offsetHours < 0 {
		sign = "-"
		offsetHours = -offsetHours
	}
	return fmt.Sprintf("Etc/GMT%s%d", sign, int(offsetHours))
}

// ─── Haversine distance (km) ──────────────────────────────────────────────────

// HaversineKm returns the great-circle distance in kilometres.
func HaversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dphi := (lat2 - lat1) * math.Pi / 180
	dlam := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dphi/2)*math.Sin(dphi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dlam/2)*math.Sin(dlam/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
