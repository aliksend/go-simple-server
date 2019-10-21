package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/novalagung/go-eek"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

type ServiceConfig struct {
	ServiceName string `json:"service_name"`
	Routes      map[string]struct {
		Imports []string `json:"imports"`
		Code    string   `json:"code"`
	} `json:"routes"`
}

type Call struct {
	Route string      `json:"route"`
	Body  interface{} `json:"body"`
}

func main() {
	data, err := ioutil.ReadFile("/config.json")
	if err != nil {
		log.Fatalf("unable to read config: %v", err)
	}
	serviceConfig := new(ServiceConfig)
	err = json.Unmarshal(data, &serviceConfig)
	if err != nil {
		log.Fatalf("unable to unmarshal config: %v", err)
	}

	// gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	var handlerConfig map[string]interface{}
	var calls []Call
	recordCall := func(method string, path string, body interface{}) {
		calls = append(calls, Call{
			Route: fmt.Sprintf("%s %s", method, path),
			Body:  body,
		})
	}

	router.POST("/__config", func(c *gin.Context) {
		if c.Request.ContentLength == 0 {
			handlerConfig = nil
		} else {
			handlerConfig = make(map[string]interface{})
			err := c.BindJSON(&handlerConfig)
			if err != nil {
				log.Println("!! error on parsing json:", err)
				c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
		}
		log.Printf(">> config updated: %#v", handlerConfig)
		c.String(http.StatusOK, "OK")
	})
	router.POST("/__reset_calls", func(c *gin.Context) {
		calls = nil
		log.Printf(">> calls reset")
		c.String(http.StatusOK, "OK")
	})
	router.GET("/__calls", func(c *gin.Context) {
		c.JSON(http.StatusOK, calls)
	})

	for route, processor := range serviceConfig.Routes {
		vm := eek.New()
		vm.SetName(route)
		vm.SetBaseBuildPath("./eek/" + serviceConfig.ServiceName)
		vm.ImportPackage("fmt")
		vm.ImportPackage("log")
		vm.ImportPackage("io/ioutil")
		vm.ImportPackage("encoding/json")
		vm.ImportPackage("github.com/gin-gonic/gin")
		for _, packageName := range processor.Imports {
			vm.ImportPackage(packageName)
		}
		vm.DefineVariable(eek.Var{Name: "C", Type: "*gin.Context"})
		vm.DefineVariable(eek.Var{Name: "RecordCall", Type: "func(method string, path string, body interface{})"})
		vm.DefineVariable(eek.Var{Name: "Config", Type: "map[string]interface{}"})
		vm.DefineFunction(eek.Func{
			Name: "BindJsonBody",
			BodyFunction: `
				func(dst interface{}) error {
					defer C.Request.Body.Close()
					if C.Request.ContentLength == 0 {
						log.Printf(">> request with empty body")
						RecordCall(C.Request.Method, C.Request.URL.Path, nil)
					} else {
						body, err := ioutil.ReadAll(C.Request.Body)
						if err != nil {
							return fmt.Errorf("unable to read body: %v", err)
						}
						err = json.Unmarshal(body, dst)
						if err != nil {
							return fmt.Errorf("unable to parse body: %v", err)
						}
						log.Printf(">> request with body %s", body)
						RecordCall(C.Request.Method, C.Request.URL.Path, string(body))
					}
					return nil
				}
			`,
		})
		vm.PrepareEvaluation(processor.Code)
		err = vm.Build()
		if err != nil {
			log.Fatalf("unable to build processor: %v", err)
		}

		routeParams := strings.SplitN(route, " ", 2)

		router.Handle(routeParams[0], routeParams[1], func(c *gin.Context) {
			response, err := vm.Evaluate(eek.ExecVar{"C": c, "Config": handlerConfig, "RecordCall": recordCall})
			if err != nil {
				c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("unable to run processor: %v", err))
				return
			}
			log.Printf(">> response to %v %v: %v", routeParams[0], routeParams[1], response)
			c.JSON(http.StatusOK, response)
		})
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	err = router.Run(":" + port)
	if err != nil {
		log.Fatalf("unable to run router: %v", err)
	}
}
