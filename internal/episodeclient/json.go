package episodeclient

import "github.com/sirerun/serenity/internal/episodejson"

func strictJSON(b []byte) bool { return episodejson.Valid(b) }
