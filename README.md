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
- Run scripts on package upload, and pre/post repository regeneration
- pool layout is used, with configurable groupings
- A git-inspired sha1 object store is used on the backend, with
  hard links to keep disk usage down
- Seems fast so far
- See the github issues list for future plans

## Mis(sing)-Features

- Sources, Content-?, Trnslations not  currently handled
- Only a single release is supported
- All versions are currently kept, (this will be configurable)
- Garbage collection on the store needs re-implementing, failed
  and timeout out upload will leak garbage to disk
- The objects in the blob store are likely to change
- Cacheing into the blob store may be a bit agressive, extra
  copies of the metadata can be avoided.
- No current means of reviewin the logs

## Example

sources.list.d/test.list:
```
deb http://localhost:3000/repo /
```

To start godinstall:

```
$ godinstall -repo-base ./testrepo \
           -gpg-privring ~/.gnupg/secring.gpg \
           -gpg-pubring ~/.gnupg/pubring.gpg \
           -signer-email tcolgate@gmail.com \
           -accept-lone-debs
```

If you do not want package validation, or repository signing, you can
ignore the gpg settings

```
$ godinstall -repo-base ./testrepo \
             -validate-changes=false \
             -validate-debs=false
```

To upload a package, either upload all the files and the changes file in one PUT: 
```
curl -v -c cookie.jar  -XPOST -F 'debfiles=@woot.changes' -F 'debfiles=@collectd-core_5.4.0-3_amd64.deb' -F 'debfiles=@collectd_5.4.0-3_amd64.deb'  http://localhost:3000/upload/$SESSION
```

Or upload the changes file, and then upload the individual files. As Session ID is returned in a JSON response, and in a cookie
```
# This just 'parses' the json :)
SESSION=`curl -XPOST -F 'debfiles=@woot.changes' http://localhost:3000/upload  | json_pp | grep SessionId | awk '{print $3}' | awk -F\" '{print $2}'`
curl -v -XPOST -F 'debfiles=@collectd-core_5.4.0-3_amd64.deb' http://localhost:3000/upload/$SESSION
curl -v -XPOST -F 'debfiles=@collectd_5.4.0-3_amd64.deb'  http://localhost:3000/upload/$SESSION
```

