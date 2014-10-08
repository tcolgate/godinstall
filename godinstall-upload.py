#!/usr/bin/python

import sys
import json
import requests

if len(sys.argv) != 3:
  usage()
  sys.exit(1)

baseurl = sys.argv[1]
changesfile = sys.argv[2]


print "Uploading changes file: " + changesfile
controlFile = {'debfiles': open(changesfile, 'rb')}
r = requests.post(baseurl, files=controlFile)

respData = json.loads(r.raw.read())
sessionID = respData["Message"]["SessionId"]

for f in respData["Message"]["Changes"]["Files"]:
  file = {'debfiles': open(f, 'rb')}
  r = requests.post(baseurl + "/" + sessionID, files=file)
  print "Uploading file: " + f

if r.status_code != 200:
  print "Upload failed: " + r.raw
  sys.exit(1)
