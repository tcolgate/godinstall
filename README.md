# GoDInstall

GoDInstall is intended to be a dynamically maintainable apt repository server

The primary motivation is to provide confirmation of package upload (and
availability), along with optional validation of signed changes and debs.

## Features

- Synchronous confirmation of repository regeneration.
- Instant feedback on all failures
- Files can be uploaded a few at a time, or all in one go
- Signing of InReleases files
- Verification of hashes and signatures in changes files
- Optional verification of debsigs signed packages
- Allow upload of lone deb packages, without changes file
- Optionally only verify debsigs signatures on lone uploads
- Control the number of version and revisions retained (see Pruning)
- Run scripts on package upload, and pre/post repository regeneration
- pool layout is used, with configurable groupings
- A git-inspired sha1 object store is used on the backend, with
  hard links to keep disk usage down
- Seems fast so far
- See the github issues list for future plans

## Mis(sing)-Features

- Sources, Content-?, Trnslations not  currently handled
- The objects in the blob store are likely to change
- Only a single component is populated at present
- Package name + version + arch must be unique accross all componenets in a
  repository (not merely main + other)

## Example

sources.list.d/test.list:
```
deb http://localhost:3000/repo mydist main
```

To start godinstall:

```
$ godinstall serve -repo-base ./testrepo \
           -gpg-privring ~/.gnupg/secring.gpg \
           -gpg-pubring ~/.gnupg/pubring.gpg \
           -signer-email tcolgate@gmail.com \
           -accept-lone-debs
```

If you do not want package validation, or repository signing, you can
ignore the gpg settings

```
$ godinstall serve -repo-base ./testrepo \
             -validate-changes=false \
             -validate-debs=false
```

To upload a package, either upload all the files and the changes file in one PUT:
```
curl -v -c cookie.jar  -XPOST -F 'debfiles=@woot.changes' -F 'debfiles=@collectd-core_5.4.0-3_amd64.deb' -F 'debfiles=@collectd_5.4.0-3_amd64.deb'  http://localhost:3000/dists/mydist/upload/$SESSION
```

Or upload the changes file, and then upload the individual files. As Session ID is returned in a JSON response, and in a cookie
```
# This just 'parses' the json :)
SESSION=`curl -XPOST -F 'debfiles=@woot.changes' http://localhost:3000/dists/mydist/upload  | json_pp | grep SessionId | awk '{print $3}' | awk -F\" '{print $2}'`
curl -v -XPOST -F 'debfiles=@collectd-core_5.4.0-3_amd64.deb' http://localhost:3000/dists/mydist/upload/$SESSION
curl -v -XPOST -F 'debfiles=@collectd_5.4.0-3_amd64.deb'  http://localhost:3000/dists/mydist/upload/$SESSION
```

## Package Pruning

You can limit the number of version and revisions of a package that will be presented in the
archive indexes. It should be noted that these packages are not removed from the objects store, they
are removed from the current version of the index, but can be accessed via the archive history (no
UI is present for that at the moment). This means disk space is not freed. In order to free disk space
the history of the archive must be trimmed (not currenlty implemented), garbage collection will then
remove any uneeded items from the archive.

In order to setup pruning, use the --prune parameter. The parameter accepts a comma seperated set of rules,
each of the following form:

```
[namePattern]_V-R
```

Where:
- [namePattern] is a regex matching a pattern names
- V is the number of historical versions to keep
- R is the number of historical package revisions to keep

Both V and R can be either `*` or a number specifying the number of historical items to keep, 0 will keep
the most recent, but no historical, 2, would keep the latest + 2 historical. For Example:

```
 .*_*-*  - Would keep all version and revisions of everything, this is what mini-dinstall does by default
 .*_0-0  - Would keep only the latest version and revisions, this is what reprepro does
 .*_2-0  - Would keep the latest revision of the most recent version and the two previous versions
 .*_0-2  - Would keep the last two revisions of the latest version
```

If multiple pruning rules are given they are process from first to last, only the first matching rule is used.
Different architectures are treated as different packages, a change of epoch is handled as a new version


## Release History Trimming

By default, all updates to the repository are done non-destrutctively. Whilst a
package may be pruned, or updated (or, in the future, deleted via the api), the
change is made on a new revision of the release. All previous revisions are
kept, so no items are ever actaully lost. It will be possible to reset a
release to a point in the history, or create a new snapshot or release branch,
from a given commit that is present in the history.

Whilst this is nice, it does mean that pruning does not actually free up any
space, it simply removes the packages from being visible in the next release of
the repository.

In order to to control space, you can limit the repository to only keep a
limited number of histories of the release available. This is called Releae
History Trimming. If a release history is trimmed, the garbage collection of a
release branch will stop marking the assets for a release for retention after
a given number of releases beyond the point that the history was trimmed at.
Any items (packages, files, release index files) referred to by commits
after that piont that are not then referred to by another branch will be valid
for garbage collection.

The objects specifying the details of all previous releases are still retained,
though items they refer to will be inaccessible. Only fully accessible releases
are shown in the log by default, but the full history is availble. This allows
the full history of the repository, back to its birth, to be retained.

