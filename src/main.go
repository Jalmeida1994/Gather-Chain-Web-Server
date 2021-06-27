package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/bramvdbogaerde/go-scp/auth"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
	"github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"golang.org/x/crypto/ssh"
)

// ContentPost is the json format of the content for all post calls
type ContentPost struct {
	Author string
	Group  string
	Commit string
	IP     string
}

// create a data structure that can hold the response from the script
type scriptResponse struct {
	Response string
}

type userHandler struct {
	client *redis.Client
}

var sdk *fabsdk.FabricSDK

const ConfigFile = "./channel0000_connection_for_gosdk.yaml"

var (
	locker uint32
)

const keyPrefix = "user:"

var appIP string = os.Getenv("VM_PUBLIC_IP") + ":22"
var vmUsername string = os.Getenv("VM_USERNAME")
var vmPassword string = os.Getenv("VM_PASSWORD")
var redisHost string = os.Getenv("REDIS_HOST")
var redisPassword string = os.Getenv("REDIS_PASSWORD")

// Existing code from above
func handleRequests() {

	op := &redis.Options{Addr: redisHost, Password: redisPassword, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}, WriteTimeout: 5 * time.Second, MaxRetries: 3}
	client := redis.NewClient(op)

	ctx := context.Background()
	err := client.Ping(ctx).Err()
	if err != nil {
		log.Fatalf("failed to connect with redis instance at %s - %v", redisHost, err)
	}

	uh := userHandler{client: client}

	myRouter := mux.NewRouter().StrictSlash(true)
	// creates a new instance of a mux router
	myRouter.HandleFunc("/test", testFunc).Methods("POST")

	// admin commands
	myRouter.HandleFunc("/init", initNet).Methods("POST")
	myRouter.HandleFunc("/clear", uh.clearNet).Methods("POST")
	myRouter.HandleFunc("/history", historyNet).Methods("POST")

	// student commands
	myRouter.HandleFunc("/creategroup", createGrp).Methods("POST")
	myRouter.HandleFunc("/registernumber", uh.registerNr).Methods("POST")
	myRouter.HandleFunc("/users/{Author}", uh.getUser).Methods("GET")
	myRouter.HandleFunc("/push", pushHash).Methods("POST")

	// finally, instead of passing in nil, we want
	// to pass in our newly created router as the second
	// argument
	log.Fatal(http.ListenAndServe(":8010", myRouter))
}

func initNet(w http.ResponseWriter, r *http.Request) {
	// get the body of our POST request
	// unmarshal this into a new Article struct
	// append this to our Articles array.
	reqBody, err := ioutil.ReadAll(r.Body)

	log.Println("Reached function")

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var cp ContentPost
	json.Unmarshal(reqBody, &cp)

	password := cp.Author
	if password != vmPassword {
		log.Println("Password given: " + password)
		log.Println("Correct password: " + password)
		http.Error(w, "Wrong Password", http.StatusForbidden)
		return
	}

	config := &ssh.ClientConfig{
		User: vmUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(vmPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", appIP, config)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// TODO: check if user is admin
	runCommand("sudo /var/lib/waagent/custom-script/download/0/project/bloc-server/commands/init.sh "+cp.Author+" "+cp.Group+" "+cp.Commit, conn, w)

	// Create a new SCP client
	scpClient := scp.NewClient(appIP, config)

	// Connect to the remote server
	err := scpClient.Connect()
	if err != nil {
		fmt.Println("Couldn't establish a connection to the remote server ", err)
		return
	}

	// Open a file we can transfer to the remote container.
	f, _ := os.Open("./data/input.txt")
	defer f.Close()

	// Create a local file to write to.
	f, err := os.OpenFile(ConfigFile, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		t.Errorf("Couldn't open the output file")
	}
	defer f.Close()

	// Use a file name with exotic characters and spaces in them.
	// If this test works for this, simpler files should not be a problem.
	err = client.CopyFromRemote(f, "/path/channel0000_connection_for_gosdk.yaml")
	if err != nil {
		t.Errorf("Copy failed from remote")
	}

	err = Initialize()
	if err != nil {
		panic(err)
	}

}

func Initialize() error {
	var err error
	// Initialize the SDK with the configuration file
	sdk, err = fabsdk.New(config.FromFile(ConfigFile))
	if err != nil {
		return fmt.Errorf("failed to create sdk: %v", err)
	}
	fmt.Println("Initialization Successful")
	return nil
}

func (uh userHandler) clearNet(w http.ResponseWriter, r *http.Request) {
	// get the body of our POST request
	// unmarshal this into a new Article struct
	// append this to our Articles array.
	reqBody, err := ioutil.ReadAll(r.Body)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var cp ContentPost
	json.Unmarshal(reqBody, &cp)

	password := cp.Author
	if password != vmPassword {
		log.Println("Password given: " + password)
		log.Println("Correct password: " + password)
		http.Error(w, "Wrong Password", http.StatusForbidden)
		return
	}

	_, err = uh.client.FlushAll(r.Context()).Result()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	config := &ssh.ClientConfig{
		User: vmUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(vmPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", appIP, config)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// TODO: check if user is admin
	runCommand("sudo /var/lib/waagent/custom-script/download/0/project/bloc-server/commands/clear.sh", conn, w)
}

func historyNet(w http.ResponseWriter, r *http.Request) {
	// get the body of our POST request
	// unmarshal this into a new Article struct
	// append this to our Articles array.
	reqBody, _ := ioutil.ReadAll(r.Body)

	var cp ContentPost
	json.Unmarshal(reqBody, &cp)

	config := &ssh.ClientConfig{
		User: vmUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(vmPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", appIP, config)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	runCommand("sudo /var/lib/waagent/custom-script/download/0/project/bloc-server/commands/gethistory.sh "+cp.Group, conn, w)
}

func createGrp(w http.ResponseWriter, r *http.Request) {
	// get the body of our POST request
	// unmarshal this into a new Article struct
	// append this to our Articles array.
	reqBody, _ := ioutil.ReadAll(r.Body)

	var cp ContentPost
	json.Unmarshal(reqBody, &cp)

	config := &ssh.ClientConfig{
		User: vmUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(vmPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", appIP, config)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	runCommand("sudo /var/lib/waagent/custom-script/download/0/project/bloc-server/commands/createchannel.sh "+cp.Author+" "+cp.Group+" "+cp.Commit, conn, w)
}

func (uh userHandler) registerNr(w http.ResponseWriter, r *http.Request) {
	// get the body of our POST request
	// unmarshal this into a new Article struct
	// append this to our Articles array.
	reqBody, err := ioutil.ReadAll(r.Body)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var u map[string]interface{}
	err = json.Unmarshal([]byte(reqBody), &u)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userid := u["Author"].(string)
	_, err = uh.client.HSet(r.Context(), keyPrefix+userid, u).Result()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (uh userHandler) getUser(w http.ResponseWriter, r *http.Request) {
	userid := mux.Vars(r)["Author"]
	info, err := uh.client.HGetAll(r.Context(), keyPrefix+userid).Result()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(info) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(info)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		w.Header().Del("Content-Type")
	}
}

func pushHash(w http.ResponseWriter, r *http.Request) {
	// get the body of our POST request
	// unmarshal this into a new Article struct
	// append this to our Articles array.
	reqBody, _ := ioutil.ReadAll(r.Body)

	var cp ContentPost
	json.Unmarshal(reqBody, &cp)

	//prepare contexts
	studentUserChannelContext := sdk.ChannelContext(cp.Group, fabsdk.WithUser(cp.Author), fabsdk.WithOrg("student.com"))

	config := &ssh.ClientConfig{
		User: vmUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(vmPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", appIP, config)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	runCommand("sudo /var/lib/waagent/custom-script/download/0/project/bloc-server/commands/push.sh "+cp.Author+" "+cp.Group+" "+cp.Commit, conn, w)
}

func testFunc(w http.ResponseWriter, r *http.Request) {
	// get the body of our POST request
	// unmarshal this into a new Article struct
	// append this to our Articles array.
	reqBody, _ := ioutil.ReadAll(r.Body)
	log.Println(reqBody)

	var cp ContentPost
	json.Unmarshal(reqBody, &cp)

	config := &ssh.ClientConfig{
		User: vmUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(vmPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", appIP, config)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	log.Println(cp)
	log.Println(cp.Author)
	log.Println(cp.Group)
	log.Println(cp.Commit)

	// Call Run method with command you want to run on remote server.
	runCommand("sudo /var/lib/waagent/custom-script/download/0/project/bloc-server/commands/test.sh "+cp.Author+" "+cp.Group+" "+cp.Commit, conn, w)
}

func runCommand(cmd string, conn *ssh.Client, w http.ResponseWriter) {
	if !atomic.CompareAndSwapUint32(&locker, 0, 1) {
		log.Println("Locked out")
		w.WriteHeader(500)
		panic("Blockchain network being used, try again next time")
	}
	defer atomic.StoreUint32(&locker, 0)

	sess, err := conn.NewSession()
	if err != nil {
		panic(err)
	}
	defer sess.Close()

	results, err := sess.Output(cmd)
	if err != nil {
		log.Println(err)
		w.WriteHeader(500)
		panic("Can't run remote command: " + err.Error())
	}

	// convert results into string and populate an instance of
	// the scriptResponse struct
	response := scriptResponse{string(results)}
	// encode response into JSON and deliver back to user
	encoder := json.NewEncoder(w)
	err = encoder.Encode(response)
	if err != nil {
		log.Println(err)
		w.WriteHeader(500)
		panic("Can't return remote command output: " + err.Error())
	}
}

func main() {
	port := 8010
	log.Printf("Starting webserver on port %d\n", port)
	handleRequests()
}
