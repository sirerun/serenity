// Package plans is the versioned source of truth for the hosted offer.
package plans

type Plan struct {
	ID           string `json:"id"`
	MonthlyCents int64  `json:"monthly_cents"`
	Brains       int64  `json:"brains"`
	Memories     int64  `json:"memories"`
	Writes       int64  `json:"writes"`
	Recalls      int64  `json:"recalls"`
	InputTokens  int64  `json:"input_tokens"`
	StorageBytes int64  `json:"storage_bytes"`
}

var V1 = []Plan{
	{"free", 0, 1, 1000, 500, 10000, 100000, 100000000},
	{"builder", 1900, 3, 10000, 5000, 100000, 1000000, 1000000000},
	{"scale", 4900, 10, 50000, 20000, 300000, 4000000, 5000000000},
}

func Get(id string) Plan {
	for _, p := range V1 {
		if p.ID == id {
			return p
		}
	}
	return V1[0]
}
