# GoDInstall

GoDInstall is intended to be a dynamically maintainable apt repository server

The primary motivation is to provide confirmation of package upload (and
availability), along with optional validation of signed changes and debs.

Apt-ftparchive is used to create the repository itself.

At present, you must provide an apt-ftparchive configuration. This configruation is not parsed,
and you need to also provide the pool and repository bases to godinstall.

The follwoing configuration has been tested:

aftp.conf:

```
Dir {
  ArchiveDir "/home/youruser/testrepo";
  CacheDir "/home/youruser/testrepo";
};

Default {
  Packages::Compress ". gzip bzip2";
  Sources::Compress ". gzip bzip2";
  Contents::Compress ". gzip bzip2";
};

TreeDefault {
  BinCacheDB "packages-$(SECTION)-$(ARCH).db";
  Directory "pool/$(SECTION)";
  Packages "$(DIST)/$(SECTION)/binary-$(ARCH)/Packages";
  SrcDirectory "pool/$(SECTION)";
  Sources "$(DIST)/$(SECTION)/source/Sources";
  Contents "$(DIST)/Contents-$(ARCH)";
};

Default {
  Packages {
    Extensions ".deb";
  };
};

Tree "dists/wheezy" {
    Sections "main";
    Architectures "amd64 i386 all source";
}
```

sources.list.d/test.list:
```
deb http://localhost:3000/repo wheezy main
```

To start godinstall:

```
$ godinstall -pool-base /home/tristan/testrepo/pool/main \
             -repo-base /home/tristan/testrepo \
             -config ~/aftp.conf \
             -tmp-dir ~/tmp \
             -rel-config ~/release.conf \
             -gpg-privring ~/.gnupg/secring.gpg \
             -gpg-pubring ~/.gnupg/pubring.gpg \
             -signer-id tcolgate@gmail.com
```

To upload a package, you upload the changes file, and then upload the individual files. You can upload the content files one at a time, or in batches, or on big batch

```
SESSION=`curl -q -c cookie.jar  -XPOST -F 'debfiles=@woot.changes' http://localhost:3000/package/upload   | awk -F\" '{print $4}'`
curl -v -c cookie.jar  -XPUT -F 'debfiles=@collectd-core_5.4.0-3_amd64.deb' -F 'debfiles=@collectd_5.4.0-3_amd64.deb'  http://localhost:3000/package/upload/$SESSION
```

