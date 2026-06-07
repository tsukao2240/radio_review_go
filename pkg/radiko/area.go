package radiko

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

var stationAreaMap = map[string]string{
	"HBC": "JP1", "STV": "JP1", "AIR-G": "JP1", "NORTHWAVE": "JP1",
	"RAB": "JP2",
	"IBC": "JP3", "FMI": "JP3",
	"TBC": "JP4", "DATEFM": "JP4",
	"ABS": "JP5", "AFM": "JP5",
	"YBC": "JP6",
	"RFC": "JP7", "FMF": "JP7",
	"JOQR": "JP13", "TBS": "JP13", "JORF": "JP13", "INT": "JP13",
	"J-WAVE": "JP13", "HOUSOU-DAIGAKU": "JP13",
	"QRR": "JP13", "LFR": "JP13", "RN1": "JP13", "RN2": "JP13",
	"CRT": "JP12", "BAYFM78": "JP12",
	"YFM": "JP14", "FMY": "JP35",
	"NACK5": "JP11",
	"CRK":   "JP28", "RCC": "JP34", "FM-GUNMA": "JP10",
	"BSN": "JP15", "FM-NIIGATA": "JP15",
	"KNB": "JP16",
	"MRO": "JP17", "HELLO FIVE": "JP17",
	"FBC": "JP18", "FM-FUKUI": "JP18",
	"YBS": "JP19", "FM-FUJI": "JP19",
	"SBC": "JP20", "FMN": "JP20",
	"CBC": "JP23", "SF": "JP23", "ZIP-FM": "JP23", "@FM": "JP23",
	"GBS": "JP21", "FMG": "JP21",
	"SBS": "JP22", "K-MIX": "JP22",
	"MBS": "JP27", "ABC": "JP27", "OBC": "JP27",
	"FM802": "JP27", "FMO": "JP27", "FM-COCOLO": "JP27",
	"CCL": "JP28", "FMOH": "JP28", "KISS FM": "JP28",
	"KBS": "JP26", "α-STATION": "JP26",
	"WBS": "JP30", "FMW": "JP30",
	"FMNARA": "JP29", "MIE-FM": "JP24", "BBC": "JP25", "e-radio": "JP25",
	"BSS": "JP31",
	"RSK": "JP33", "FM-OKAYAMA": "JP33",
	"HFM": "JP34", "FM-FUKUYAMA": "JP34",
	"KRY": "JP35",
	"JRT": "JP36", "FMT": "JP36",
	"RNC": "JP37", "FM-KAGAWA": "JP37",
	"RNB": "JP38", "JOEU-FM": "JP38",
	"RKC": "JP39", "HI-SIX": "JP39",
	"KBC": "JP40", "RKB": "JP40", "LOVE-FM": "JP40", "FM-FUKUOKA": "JP40", "CROSS FM": "JP40",
	"STS": "JP41",
	"NBC": "JP42", "FM-NAGASAKI": "JP42",
	"RKK": "JP43", "FMK": "JP43",
	"OBS": "JP44", "FM-OITA": "JP44",
	"MRT": "JP45", "JOY-FM": "JP45",
	"MBC": "JP46", "μFM": "JP46",
	"RBC": "JP47", "ROK": "JP47", "FM-OKINAWA": "JP47", "FM21": "JP47",
}

var areaCoordinates = map[string][2]float64{
	"JP1":  {43.064615, 141.346807},
	"JP2":  {40.824308, 140.739998},
	"JP3":  {39.703619, 141.152684},
	"JP4":  {38.268837, 140.8721},
	"JP5":  {39.718614, 140.102364},
	"JP6":  {38.240436, 140.363633},
	"JP7":  {37.750299, 140.467551},
	"JP8":  {36.341811, 140.446793},
	"JP9":  {36.565725, 139.883565},
	"JP10": {36.390668, 139.060406},
	"JP11": {35.856999, 139.648849},
	"JP12": {35.605057, 140.123306},
	"JP13": {35.689487, 139.691711},
	"JP14": {35.447507, 139.642342},
	"JP15": {37.902552, 139.023095},
	"JP16": {36.695291, 137.211338},
	"JP17": {36.594682, 136.625573},
	"JP18": {36.065178, 136.221527},
	"JP19": {35.664158, 138.568449},
	"JP20": {36.651299, 138.180956},
	"JP21": {35.391227, 136.722291},
	"JP22": {34.97712, 138.383084},
	"JP23": {35.180188, 136.906565},
	"JP24": {34.730283, 136.508588},
	"JP25": {35.004531, 135.86859},
	"JP26": {35.021247, 135.755597},
	"JP27": {34.686297, 135.519661},
	"JP28": {34.691269, 135.183071},
	"JP29": {34.685334, 135.832742},
	"JP30": {34.225987, 135.167506},
	"JP31": {35.503891, 134.237736},
	"JP32": {35.472295, 133.0505},
	"JP33": {34.661751, 133.934406},
	"JP34": {34.39656, 132.459622},
	"JP35": {34.185956, 131.470649},
	"JP36": {34.065718, 134.55936},
	"JP37": {34.340149, 134.043444},
	"JP38": {33.841624, 132.765681},
	"JP39": {33.559706, 133.531079},
	"JP40": {33.606576, 130.418297},
	"JP41": {33.249442, 130.299794},
	"JP42": {32.744839, 129.873756},
	"JP43": {32.789827, 130.741667},
	"JP44": {33.238172, 131.612619},
	"JP45": {31.911096, 131.423893},
	"JP46": {31.560146, 130.557978},
	"JP47": {26.2124, 127.680932},
}

// GetAreaIDFromStationID returns the station's primary area. Unknown stations default to Tokyo.
func GetAreaIDFromStationID(stationID string) string {
	if areaID, ok := stationAreaMap[strings.TrimSpace(stationID)]; ok {
		return areaID
	}
	return "JP13"
}

func generateGPSLocation(areaID string) string {
	coords, ok := areaCoordinates[areaID]
	if !ok {
		coords = areaCoordinates["JP13"]
	}

	lat := coords[0] + jitter()
	lng := coords[1] + jitter()
	return fmt.Sprintf("%.6f,%.6f,gps", lat, lng)
}

func jitter() float64 {
	n, err := rand.Int(rand.Reader, big.NewInt(501))
	if err != nil {
		return 0
	}
	return float64(n.Int64()-250) / 10000
}
