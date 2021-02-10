// This small program is just a small web server created in static mode
// in order to provide the smallest docker image possible

package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

var (
	// Def of flags
	configFile               = flag.String("config", "config.yaml", "The yaml file with config")
	portPtr                  = flag.Int("port", 8043, "The listening port")
	context                  = flag.String("context", "", "The 'context' path on which files are served, e.g. 'doc' will serve the files at 'http://localhost:<port>/doc/'")
	context2                 = flag.String("context2", "", "The 'context' path on which files are served, e.g. 'doc' will serve the files at 'http://localhost:<port>/doc/'")
	context3                 = flag.String("context3", "", "The 'context' path on which files are served, e.g. 'doc' will serve the files at 'http://localhost:<port>/doc/'")
	basePath                 = flag.String("path", "/srv/http", "The path for the static files")
	basePath2                = flag.String("path2", "", "The path for the static files")
	fallbackPath             = flag.String("fallback", "", "Default fallback file. Either absolute for a specific asset (/index.html), or relative to recursively resolve (index.html)")
	headerFlag               = flag.String("append-header", "", "HTTP response header, specified as `HeaderName:Value` that should be added to all responses.")
	basicAuth                = flag.Bool("enable-basic-auth", false, "Enable basic auth. By default, password are randomly generated. Use --set-basic-auth to set it.")
	healthCheck              = flag.Bool("enable-health", false, "Enable health check endpoint. You can call /health to get a 200 response. Useful for Kubernetes, OpenFaas, etc.")
	setBasicAuth             = flag.String("set-basic-auth", "", "Define the basic auth. Form must be user:password")
	defaultUsernameBasicAuth = flag.String("default-user-basic-auth", "gopher", "Define the user")
	sizeRandom               = flag.Int("password-length", 16, "Size of the randomized password")
	logRequest               = flag.Bool("enable-logging", false, "Enable log request")
	httpsPromote             = flag.Bool("https-promote", false, "All HTTP requests should be redirected to HTTPS")
	headerConfigPath         = flag.String("header-config-path", "/config/headerConfig.json", "Path to the config file for custom response headers")

	username string
	password string
)

func parseHeaderFlag(headerFlag string) (string, string) {
	if len(headerFlag) == 0 {
		return "", ""
	}
	pieces := strings.SplitN(headerFlag, ":", 2)
	if len(pieces) == 1 {
		return pieces[0], ""
	}
	return pieces[0], pieces[1]
}

var gzPool = sync.Pool{
	New: func() interface{} {
		w := gzip.NewWriter(ioutil.Discard)
		return w
	},
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func handleReq(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *httpsPromote && r.Header.Get("X-Forwarded-Proto") == "http" {
			http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusMovedPermanently)
			if *logRequest {
				log.Println(301, r.Method, r.URL.Path)
			}
			return
		}

		if *logRequest {
			log.Println(r.Method, r.URL.Path)
		}

		h.ServeHTTP(w, r)
	})
}

type endpoint struct {
	context  string
	basePath string
	handler  http.Handler
}

func parseEndpoints() ([]endpoint, error) {
	retval := []endpoint{}

	viper.SetConfigFile(*configFile)
	viper.SetConfigType("yaml")
	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("fatal error config file: %v \n", err)
	}

	fmt.Println("VIPER FALLBACK = ", viper.GetString("fallback"))
	fmt.Println("One:", viper.Get("endpoints.0"))
	endpoints := viper.Get("endpoints").([]interface{})
	fmt.Println(endpoints)
	for k, v := range endpoints {
		fmt.Println(k, v)
		vMap := v.(map[interface{}]interface{})
		for _, v2 := range vMap {
			fmt.Println("KV", v2)
			m2 := v2.(map[interface{}]interface{})
			ep := endpoint{
				context:  m2["context"].(string),
				basePath: m2["basePath"].(string),
			}
			retval = append(retval, ep)
		}
	}

	return retval, nil

}

func main() {

	flag.Parse()

	endpoints, err := parseEndpoints()
	if err != nil {
		panic(err)
	}

	fmt.Println("Endpoints: ", endpoints)
	// sanity check
	if len(*setBasicAuth) != 0 && !*basicAuth {
		*basicAuth = true
	}

	port := ":" + strconv.FormatInt(int64(*portPtr), 10)

	for i := range endpoints {
		err := makeHandler(&endpoints[i])
		if err != nil {
			panic(err)
		}
	}

	// if *basicAuth {
	// 	log.Println("Enabling Basic Auth")
	// 	if len(*setBasicAuth) != 0 {
	// 		parseAuth(*setBasicAuth)
	// 	} else {
	// 		generateRandomAuth()
	// 	}
	// 	handler = authMiddleware(handler)
	// }

	// headerConfigValid := initHeaderConfig(*headerConfigPath)
	// if headerConfigValid {
	// 	handler = customHeadersMiddleware(handler)
	// }

	// // Extra headers.
	// if len(*headerFlag) > 0 {
	// 	header, headerValue := parseHeaderFlag(*headerFlag)
	// 	if len(header) > 0 && len(headerValue) > 0 {
	// 		fileServer := handler
	// 		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.Header().Set(header, headerValue)
	// 			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
	// 				fileServer.ServeHTTP(w, r)
	// 			} else {
	// 				w.Header().Set("Content-Encoding", "gzip")
	// 				gz := gzPool.Get().(*gzip.Writer)
	// 				defer gzPool.Put(gz)

	// 				gz.Reset(w)
	// 				defer gz.Close()
	// 				fileServer.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, Writer: gz}, r)
	// 			}

	// 		})
	// 	} else {
	// 		log.Println("appendHeader misconfigured; ignoring.")
	// 	}
	// }

	// if *healthCheck {
	// 	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
	// 		fmt.Fprintf(w, "Ok")
	// 	})
	// }

	for _, e := range endpoints {
		http.Handle(e.basePath, e.handler)
	}

	log.Printf("Listening at 0.0.0.0%v...", port)
	log.Fatalln(http.ListenAndServe(port, nil))
}

func makeHandler(e *endpoint) error {

	var fileSystem http.FileSystem = http.Dir(e.basePath)

	if *fallbackPath != "" {
		fileSystem = fallback{
			defaultPath: *fallbackPath,
			fs:          fileSystem,
		}
	}
	e.handler = handleReq(http.FileServer(fileSystem))

	pathPrefix := "/"
	if len(e.context) > 0 {
		pathPrefix = "/" + e.context + "/"
		e.handler = http.StripPrefix(pathPrefix, e.handler)
	}

	return nil

}
