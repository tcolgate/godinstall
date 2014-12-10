#!/usr/bin/python

import sys
import os
import os.path
import json
import requests

if len(sys.argv) != 3:
  usage()
  sys.exit(1)

baseurl = sys.argv[1]
changesfile = sys.argv[2]

workingDir = os.path.dirname(changesfile)
os.chdir(workingDir)

print "Uploading changes file: " + changesfile
controlFile = {'debfiles': open(changesfile, 'rb')}
r = requests.post(baseurl, files=controlFile)

respData = json.loads(r.content)
sessionID = respData["Message"]["SessionID"]

for f in respData["Message"]["Expecting"]:
  file = {'debfiles': open(f, 'rb')}
  r = requests.post(baseurl + "/" + sessionID, files=file)
  print "Uploading file: " + f
  print r.content

if r.status_code != 200:
  print "Upload failed: " + r.content
  sys.exit(1)
