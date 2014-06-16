package main

var test_server1 = &AptServer{
	MaxReqs:         2,
	RepoBase:        "tests/repo",
	PoolBase:        "tests/repo/pool",
	TmpDir:          "tests/tmp",
	CookieName:      "testrepo-cookie",
	TTL:             3,
	ValidateChanges: true,
	ValidateDebs:    true,
	AftpPath:        "tests/apt-ftpserver",
	AftpConfig:      "tests/apt.cfg",
	ReleaseConfig:   "tests/release.cfg",
	PostUploadHook:  "",
	PreAftpHook:     "",
	PostAftpHook:    "",
	PoolPattern:     nil,
	PubRing:         nil,
	PrivRing:        nil,
	SignerId:        nil,
}
