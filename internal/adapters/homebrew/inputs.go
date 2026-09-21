package homebrew

import (
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"strconv"
)

type nativeInputs struct {
	Schema     int               `json:"schema" required:"true"`
	Recipes    []formulaMetadata `json:"recipes" required:"true"`
	Candidates []formulaMetadata `json:"candidates" required:"true"`
	Targets    []string          `json:"targets" required:"true"`
	Operation  string            `json:"operation" required:"true"`
}
type inspectedCandidate struct {
	Name                 string        `json:"name" required:"true"`
	CachePath            string        `json:"cachePath" required:"true"`
	EmbeddedRecipeSHA256 domain.Digest `json:"embeddedRecipeSHA256" required:"true"`
}
type inspectionDocument struct {
	Schema     int                  `json:"schema" required:"true"`
	Candidates []inspectedCandidate `json:"candidates" required:"true"`
}

func nativeBottleName(f formulaMetadata) string {
	version := f.Version
	if f.Revision > 0 {
		version += "_" + strconv.Itoa(f.Revision)
	}
	rebuild := ""
	if f.Rebuild > 0 {
		rebuild = "." + strconv.Itoa(f.Rebuild)
	}
	return f.Name + "--" + version + ".arm64_tahoe.bottle" + rebuild + ".tar.gz"
}
