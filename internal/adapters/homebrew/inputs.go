package homebrew

import (
	"strconv"
)

type collectionInputs struct {
	Schema     int               `json:"schema" required:"true"`
	Candidates []formulaMetadata `json:"candidates" required:"true"`
	Targets    []string          `json:"targets" required:"true"`
	Operation  string            `json:"operation" required:"true"`
}

func bottleName(f formulaMetadata) string {
	version := f.Version
	if f.Revision > 0 {
		version += "_" + strconv.Itoa(f.Revision)
	}
	rebuild := ""
	if f.Rebuild > 0 {
		rebuild = "." + strconv.Itoa(f.Rebuild)
	}
	return f.Name + "--" + version + "." + f.BottleTag + ".bottle" + rebuild + ".tar.gz"
}
