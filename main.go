package main

	//"crypto/md5"
  //"github.com/stapelberg/godebiancontrol"

import (
	"bytes"
	"net/http"
	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/armor"
	"code.google.com/p/go-uuid/uuid"
  "github.com/gorilla/mux"
	"fmt"
	"log"
	"time"
	"os"
)

const encryptedMessage = `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: GnuPG v1

mQENBFNxLd8BCAC9bnW3KfppUfGZsRP6t04bsdUVDYWha6XZs6eJcRJ50IzyU/j7
2iL5Mk5f/XejzQRB1rbg1D7EO4ceGi3nbe1AFj1tS6zbeOWI0f1zyvv2hwKTxjbr
nH8llYb2fVKgrPdTRHdGePLOD6Re0qh39D87TIBlwlyLBHg1PSzUQ8N5encLxI/1
9kG9zucl4fTsxN5lQbfkaqhcvWYGsgSvUDDa75Mcy51dI3SlDtNnqrwLCpa/YcxU
0kaV4ctnPWRj8B5m+xpgq4wzNAcQ3eu2ixNQd/SOBFU4/t/REx2YnLJ3AxRbuBRa
bp4sy1f++b4wqpZyUiDueYeGsBKuFu6FVksDABEBAAG0JlRyaXN0YW4gKHRlc3Rl
cikgPHRyaXN0YW5AZXhhbXBsZS5jb20+iQE4BBMBAgAiBQJTcS3fAhsDBgsJCAcD
AgYVCAIJCgsEFgIDAQIeAQIXgAAKCRArHZwTb/8tOpVPCACDMnXUq38QyhfZP0oK
geUrLXwmBbnR2AZgNmubv0XUXHDQ5LIV5Q/OPQa4B1fVh6+R4a6F0a1XS73W4fqD
kLxBUlX4z/SHFdGrprhCGNw5h9dZPesPgC0Q6fiXZNOJqV0ZbWnfHe20CjvCuabd
fOTpNHA6Bu46y+RKtARHLGmPWfZjvCCF8dKiYJp9YjQ1WPTYrCRqq6ad1xB8MCZN
maCfn+snPbjpALiNO/PO6O92O8uQfgfyhwZYndbe/vqTejbSL+6s677daj/aGL6q
mB3fysQ+1mNcMTiL3WpSJhciKhctpOlVm6W4AEs+XW3uy9SLCCQlk8bl4Ldnk9Fa
ZK1tuQENBFNxLd8BCAC8Slw4oRDhGJTWgEDyS09Fm04gHOvulzYZJigyMok7OWJl
FgS2r2lZEv4Ym5aJzaOBZmr+QZTjZWVd4HdTdvqN4C6gJTk6/kPMaRM/sor0GPjw
M48u+xCxjVbVlVt58yUc+3WAhNWyv0nEW73SS3uy5MkyMoYPUJ81u/ubI3BOWAtl
Mf/zicMc3tgoe0/IfhTck2lJyzrvLk7pC2fzOodMutO3RmbbdTeYvVeV8p9PZ8bf
u7b/mfqfFJZ8F5QCIUNR+atrD34rI82WiYFHw4MVWIjBImhkYO+LaV4wDCv2zGhT
U2Lh6tb7rjYgaLxw0ItIFABaLEW6WldqLqHMkzvTABEBAAGJAR8EGAECAAkFAlNx
Ld8CGwwACgkQKx2cE2//LTrXmgf/Wbt1wMqXISnH+4lcAPDE0T46Nb5j0jloU99/
rKkJ+8CVKajY6Js8PniRwk/ggQ0w/OAWnuDW2Xzve79O8qDP5HknyLiui+EJkn6O
gZ3ediuXxKbXOZwcaQ3Xp1yH3Ax1XjT6F3wN44yQfRpiRAiqophxZ8LwvZ2B4u5J
ssDUnTRQ5m4RRW7btddcaacAQk2X9TgcAytAopOq98OMoQUl8X8eSeNax4c4xi51
PvRzxOjmCBraDxByn9iUgWZ/Ck5IBTAICqFf9mNsV9JTJg4rQPtZ6X1dz3Gc3/DV
Go/ntXaFa63K2EwaGOvwmXFI1G9CSZ10tulOQldvboBPu8m2wQ==
=p2l/
-----END PGP PUBLIC KEY BLOCK-----`


func TestVeriffy(){
	decbuf := bytes.NewBuffer([]byte(encryptedMessage))
	result, err := armor.Decode(decbuf)
	if err != nil {
		log.Fatal("decode ")
	}

	//md, err := openpgp.ReadMessage(result.Body, nil, func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
	md, err := openpgp.ReadArmoredKeyRing(decbuf)
	if err != nil {
		log.Fatal(err)
	}

  //fmt.Println("dec version:", result.Header["Version"])
	fmt.Println("dec type:", result.Type)
	fmt.Println("dec type:", md)
  //bytes, err := ioutil.ReadAll(md.UnverifiedBody)
	//fmt.Println("md:", string(bytes))
}


func main() {
  r :=  mux.NewRouter()
  r.HandleFunc("/", rootHandler).Methods("GET")
  r.HandleFunc("/repo", repoHandler).Methods("GET")
  r.HandleFunc("/package/upload", uploadHandler).Methods("POST", "PUT")
  http.Handle("/",r)
  http.ListenAndServe(":3000",nil)
}

func rootHandler(w http.ResponseWriter, r *http.Request){
  //params := mux.Vars(r)
  w.Write([]byte("Nothing to see here"))
}

func repoHandler(w http.ResponseWriter, r *http.Request){
  //params := mux.Vars(r)
  w.Write([]byte("Hello2"))
}

func uploadHandler(w http.ResponseWriter, r *http.Request){
  tmpDir := "/tmp"
  cookieName := "godinstall-sess"
  expire := time.Now().AddDate(0,0,1)
  cookie, err := r.Cookie(cookieName)

  if err != nil {
    log.Println(err)
    sess := uuid.New()
    cookie := http.Cookie{
      Name: cookieName,
      Value: sess,
      Expires: expire,
      HttpOnly: false,
      Path: "/package/upload"}
    http.SetCookie(w, &cookie)
    w.Write([]byte(uuid.New()))

    os.Mkdir(tmpDir + "/" + sess, os.FileMode(0755))
  } else {
    w.Write([]byte("Hello3 " + cookie.Value))
  }

}

