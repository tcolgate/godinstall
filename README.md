# GoDInstall

GoDInstall is intended to be a dynamically maintainable apt repository server

The primary motivation is to provide confirmation of package upload (and
availability), along with optional validation of signed changes and debs.

## Features

- Synchronous confirmation of repository regeneration.
- Instant feedback on all failures
- Files can be uploaded a few at a time, or all in one go
- Signing of Packages and InReleases files
- Verification of hashes and signatures in changes files
- Optional verification of debsigs signed packages
- Allow upload of lone deb packages, without changes file
- Optionally only verify debsigs signatures on lone uploads
- Run scripts on package upload, and pre/post repository regeneration

## Example

sources.list.d/test.list:
```
deb http://localhost:3000/repo /
```

To start godinstall:

```
$ godinstall -repo-base /home/tristan/testrepo \
           -tmp-dir ~/tmp \
           -store-dir ~/repostore \
           -gpg-privring ~/.gnupg/secring.gpg \
           -gpg-pubring ~/.gnupg/pubring.gpg \
           -signer-email tcolgate@gmail.com \
           -accept-lone-debs
```

If you do not want package validation, or repository signing, you can
ignore the gpg settings

```
$ godinstall -repo-base /home/tristan/testrepo \ 
             -tmp-dir ~/tmp \
             -store-dir ~/repostore \
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

