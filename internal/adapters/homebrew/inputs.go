package homebrew

import "strconv"

type collectionInputs struct {
	Schema     int               `json:"schema" required:"true"`
	Candidates []formulaMetadata `json:"candidates" required:"true"`
	Targets    []string          `json:"targets" required:"true"`
	Operation  string            `json:"operation" required:"true"`
}

// kegVersion excludes bottle rebuilds: revisions change the keg directory,
// while rebuilds identify different archives for the same keg version.
func kegVersion(version string, revision int) string {
	if revision > 0 {
		return version + "_" + strconv.Itoa(revision)
	}
	return version
}

func bottleName(f formulaMetadata) string {
	version := kegVersion(f.Version, f.Revision)
	rebuild := ""
	if f.Rebuild > 0 {
		rebuild = "." + strconv.Itoa(f.Rebuild)
	}
	return f.Name + "--" + version + "." + f.BottleTag + ".bottle" + rebuild + ".tar.gz"
}
