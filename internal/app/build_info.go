package app

import (
	"runtime/debug"

	coreversion "github.com/bnema/tmux-session-sidebar/internal/core/version"
)

var readBuildInfo = debug.ReadBuildInfo

func currentBuildMetadata() coreversion.BuildMetadata {
	info, ok := readBuildInfo()
	if !ok {
		info = nil
	}
	return coreversion.FromBuildSettings(version, commit, date, builtBy, tag, distance, dirty, buildInfoFallback(info))
}

func buildInfoFallback(info *debug.BuildInfo) coreversion.BuildInfoFallback {
	if info == nil {
		return coreversion.BuildInfoFallback{}
	}
	fallback := coreversion.BuildInfoFallback{Version: info.Main.Version}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			fallback.Revision = setting.Value
		case "vcs.time":
			fallback.Time = setting.Value
		case "vcs.modified":
			fallback.Modified = setting.Value
		}
	}
	return fallback
}
