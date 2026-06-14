package viewmodel

// MetadataKind identifies the kind of secondary session metadata being shown.
type MetadataKind string

const (
	// MetadataKindGit shows git repository metadata.
	MetadataKindGit MetadataKind = "git"
	// MetadataKindDirectory shows a filesystem directory path.
	MetadataKindDirectory MetadataKind = "directory"
	// MetadataKindAdhoc shows an ad-hoc label.
	MetadataKindAdhoc MetadataKind = "adhoc"
	// MetadataKindLoading shows a temporary loading placeholder.
	MetadataKindLoading MetadataKind = "loading"
)

// MetadataIconMode selects the icon style used by a UI adapter.
type MetadataIconMode string

const (
	// MetadataIconsNerd uses Nerd Font glyphs.
	MetadataIconsNerd MetadataIconMode = "nerd"
	// MetadataIconsASCII uses portable ASCII fallbacks.
	MetadataIconsASCII MetadataIconMode = "ascii"
)

// SessionMetadataSubline carries the optional metadata rendered below a session.
//
// Field usage depends on Kind:
//   - MetadataKindGit uses Branch, Clean, Ahead, Behind, UpstreamAhead,
//     UpstreamBehind, Staged, Modified, Deleted, Renamed, Untracked,
//     Conflicts, and UpstreamMissing.
//   - MetadataKindDirectory uses Path.
//   - MetadataKindAdhoc uses Label.
//   - MetadataKindLoading typically leaves the variant-specific fields empty.
//
// Git metadata may populate both the Ahead/Behind pair and the
// UpstreamAhead/UpstreamBehind pair at the same time when the current branch
// tracks one remote while that tracking branch is itself compared against an
// upstream branch.
type SessionMetadataSubline struct {
	// Kind identifies the metadata variant.
	Kind MetadataKind
	// SessionName is the owning session name.
	SessionName string
	// Branch is the git branch name when Kind is MetadataKindGit.
	Branch string
	// Clean reports whether the repository has no working changes.
	Clean bool
	// Ahead is the number of local commits ahead of the branch's tracking branch,
	// for example a local feature branch ahead of origin/main.
	Ahead int
	// Behind is the number of local commits behind the branch's tracking branch,
	// for example a local feature branch behind origin/main.
	Behind int
	// UpstreamAhead is the number of commits the tracking branch is ahead of a
	// configured upstream branch, for example origin/main ahead of upstream/main.
	UpstreamAhead int
	// UpstreamBehind is the number of commits the tracking branch is behind a
	// configured upstream branch, for example origin/main behind upstream/main.
	UpstreamBehind int
	// Staged is the staged file-change count.
	Staged int
	// Modified is the modified file count.
	Modified int
	// Deleted is the deleted file count.
	Deleted int
	// Renamed is the renamed file count.
	Renamed int
	// Untracked is the untracked file count.
	Untracked int
	// Conflicts is the merge-conflict count.
	Conflicts int
	// UpstreamMissing reports whether the configured upstream is absent.
	UpstreamMissing bool
	// Path is the relevant filesystem path for directory-style metadata.
	Path string
	// Label is a generic rendered label for non-git metadata.
	Label string
}
