package main

import (
    "flag"
    "io"
    "os"
    "fmt"
    "time"
    "net/http"
    "bytes"
    "strings"
    "strconv"
    "log"
    "encoding/json"
    "github.com/rs/xmux"
    "github.com/rs/xhandler"
    "golang.org/x/net/context"
)

type Registration struct {
    Id string      `json:"id"`
    Name string    `json:"name"`
    Address string `json:"address"`
    Port int       `json:"port"`
    Tags []string  `json:"tags"`
    Check          `json:"check"`
}

type Check struct {
    HTTP string      `json:"http"`
    Interval string  `json:"interval"`
    Deregister_critical_service_after string `json:"deregister_critical_service_after"`
}

type ServiceStub struct {
    Url string      `json:"url"`
    Service string  `json:"service"`
    Host string     `json:"host"`
    Port int        `json:"port"`
}

// -name <service> -port 9101 -prefix /foo,/bar
func parseArgs() (help *bool, name *string, port *int, prefix *string) {
    help = flag.Bool("help", false, "Help")
    name = flag.String("name", "sulvice", "The name of the service")
    port = flag.Int("port", 9357, "The port number this service listens")
    prefix = flag.String("prefix", "/", "Comma-sep list of /prefix resources this service serves")
    flag.Parse()
    return help, name, port, prefix
}

func registerHandler(ctx context.Context, resp http.ResponseWriter, req *http.Request) {
    host, e := os.Hostname()
    if e != nil {
        host = "localhost"
    } else {
        host = strings.Join([]string{host, "cisco.com"}, ".")
    }
    name, _ := ctx.Value("name").(string)
    port, _ := ctx.Value("port").(int)
    prefix, _ := ctx.Value("prefix").(string)
    portStr := strconv.Itoa(port)
    serviceId := strings.Join([]string{ name, "-", host, ":", portStr }, "")
    regURL := "http://localhost:8500/v1/agent/service/register"
    checkURL := strings.Join([]string{ "http://localhost:", portStr, "/health"}, "")

    var tags []string
    prefixes := strings.Split(prefix, ",")
    for _, pathPrefix := range prefixes {
        if pathPrefix == "/" {
            continue
        }
        tags = append(tags, "urlprefix-"+pathPrefix)
    }
    tags = append(tags, "api")

    input := Registration{
        Id: serviceId,
        Name: name,
        Address: host,
        Port: port,
        Tags: tags,
        Check: Check{
            HTTP: checkURL,
            Interval: "15s",
            Deregister_critical_service_after: "15s",
        },
    }

    buf := new(bytes.Buffer)
    json.NewEncoder(buf).Encode(input)
    res, err := http.Post(regURL, "application/json; charset=utf-8", buf)
    if err != nil {
        log.Fatal(err)
    } else {
        defer res.Body.Close()
        //fmt.Println("status: ", res.Status)
        fmt.Println("register: status code", res.StatusCode)
        io.Copy(os.Stdout, res.Body)
    }
}

func deregisterHandler(ctx context.Context, resp http.ResponseWriter, req *http.Request) {
    name, _ := ctx.Value("name").(string)
    regURL := strings.Join([]string{ "http://localhost:8500/v1/agent/service/deregister/", name}, "")

    //buf := new(bytes.Buffer)
    //json.NewEncoder(buf).Encode(input)
    //res, err := http.Get(regURL, "application/json; charset=utf-8", buf)
    res, err := http.Get(regURL)
    if err != nil {
        log.Fatal(err)
    } else {
        defer res.Body.Close()
        //fmt.Println("status: ", res.Status)
        fmt.Println("deregister: status code", res.StatusCode)
        io.Copy(os.Stdout, res.Body)
    }
}

func healthHandler(ctx context.Context, resp http.ResponseWriter, req *http.Request) {
    json.NewEncoder(resp).Encode(struct{}{})
    fmt.Println("healthHandler", time.Now().UTC().Format("2006-01-02 15:04:05.999999999"))
}

func rootHandler(ctx context.Context, resp http.ResponseWriter, req *http.Request) {
    name, _ := ctx.Value("name").(string)
    port, _ := ctx.Value("port").(int)
    host, err := os.Hostname()
    if err != nil {
        host = "localhost"
    }
    json.NewEncoder(resp).Encode(struct{}{})
    fmt.Println("rootHandler: (name:", name, "; port:", port, ")", host)
}

func stubHandler(ctx context.Context, resp http.ResponseWriter, req *http.Request) {
    name, _ := ctx.Value("name").(string)
    port, _ := ctx.Value("port").(int)
    hostname, err := os.Hostname()
    if err != nil {
        hostname = "localhost"
    }

    respond := ServiceStub {
        Url: req.RequestURI,
        Service: name,
        Host: hostname,
        Port: port,
    }
    if encodeErr := json.NewEncoder(resp).Encode(respond); encodeErr != nil {
        fmt.Println("stubHandler", time.Now().UTC().Format("2006-01-02 15:04:05.999999999"), encodeErr)
	} else {
        fmt.Println("stubHandler", time.Now().UTC().Format("2006-01-02 15:04:05.999999999"))
    }
}

func wrapContext(name string, port int, prefix string, f xhandler.HandlerFuncC) xhandler.HandlerFuncC {
    return xhandler.HandlerFuncC(func (ctx context.Context, w http.ResponseWriter, r *http.Request) {
        ctx = context.WithValue(ctx, "name", name)
        ctx = context.WithValue(ctx, "port", port)
        ctx = context.WithValue(ctx, "prefix", prefix)
        f(ctx, w, r)
    })
}

func main() {
    pHelp, pName, pPort, pPrefix := parseArgs()
    if *pHelp {
        flag.PrintDefaults()
        os.Exit(0);
    }

    mux := xmux.New()
    mux.GET("/", xhandler.HandlerFuncC(wrapContext(*pName, *pPort, *pPrefix, rootHandler)))
    mux.GET("/register", xhandler.HandlerFuncC(wrapContext(*pName, *pPort, *pPrefix, registerHandler)))
    mux.GET("/deregister", xhandler.HandlerFuncC(wrapContext(*pName, *pPort, *pPrefix, deregisterHandler)))
    mux.GET("/health", xhandler.HandlerFuncC(healthHandler))

    prefixes := strings.Split(*pPrefix, ",")
    for _, pathPrefix := range prefixes {
        if pathPrefix == "/" {
            continue
        }
        mux.GET(pathPrefix, xhandler.HandlerFuncC(wrapContext(*pName, *pPort, *pPrefix, stubHandler)))
    }

    log.Fatal(http.ListenAndServe(strings.Join([]string{ ":", strconv.Itoa(*pPort) }, ""),
                                  xhandler.New(context.Background(), mux)))
    //log.Fatal(http.ListenAndServeTLS(strings.Join([]string{ ":", strconv.Itoa(*pPort) }, ""),
    //                                 "/etc/consul.d/ssl/consul.cert",
    //                                 "/etc/consul.d/ssl/consul.key",
    //                                 xhandler.New(context.Background(), mux)))
}
