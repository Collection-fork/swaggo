package parserv3

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/teambition/swaggo/swaggerv3"
	yaml "gopkg.in/yaml.v2"
)

var (
	gomodInfo *GoMod
	vendor    = ""
	goPaths   = []string{}
	goRoot    = ""
)

type Module struct {
	Path    string
	Version string
}

type GoMod struct {
	Module  Module
	Go      string
	Require []Require
	Exclude []Module
	Replace []Replace
}

type Require struct {
	Path     string
	Version  string
	Indirect bool
}

type Replace struct {
	Old Module
	New Module
}

func init() {
	goPaths = filepath.SplitList(os.Getenv("GOPATH"))
	if len(goPaths) == 0 {
		panic("GOPATH environment variable is not set or empty")
	}
	goRoot = runtime.GOROOT()
	if goRoot == "" {
		panic("GOROOT environment variable is not set or empty")
	}
	cmd := exec.Command("go", "mod", "edit", "-json")
	var output bytes.Buffer
	cmd.Stdout = &output
	e := cmd.Run()
	if e != nil {
		fmt.Println("[warin]go mod command fail " + e.Error())
		return
	}
	mod := new(GoMod)
	e = json.Unmarshal(output.Bytes(), mod)
	if e != nil {
		panic("[error] json unmarshal err : " + e.Error())
	}
	gomodInfo = mod
}

// Parse the project by args
func Parse(projectPath, swaggerGo, output, t string) (err error) {
	absPPath, err := filepath.Abs(projectPath)
	if err != nil {
		return err
	}
	vendor = filepath.Join(absPPath, "vendor")

	sw := swaggerv3.New()
	sw.Openapi = "3.0.1"
	if err = doc2SwaggerV3(projectPath, swaggerGo, sw); err != nil {
		return
	}
	var (
		data     []byte
		filename string
	)

	switch t {
	case "json":
		filename = jsonFile
		data, err = json.Marshal(sw)
	case "yaml":
		filename = yamlFile
		data, err = yaml.Marshal(sw)
	default:
		err = fmt.Errorf("missing swagger file type(%s), only support in (json, yaml)", t)
	}
	if err != nil {
		return
	}
	return ioutil.WriteFile(filepath.Join(output, filename), data, 0644)
}

func doc2SwaggerV3(projectPath, swaggerGo string, sw *swaggerv3.Swagger) error {
	f, err := parser.ParseFile(token.NewFileSet(), swaggerGo, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	// Analyse API comments
	if f.Comments != nil {
		for _, c := range f.Comments {
			for _, s := range strings.Split(c.Text(), "\n") {
				switch {
				case tagTrimPrefixAndSpace(&s, appVersion):
					sw.Info.Version = s
				case tagTrimPrefixAndSpace(&s, appTitle):
					sw.Info.Title = s
				case tagTrimPrefixAndSpace(&s, appDesc):
					if sw.Info.Description != "" {
						sw.Info.Description += "<br>" + s
					} else {
						sw.Info.Description = s
					}
				case tagTrimPrefixAndSpace(&s, appTermsOfServiceURL):
					sw.Info.TermsOfService = s
				case tagTrimPrefixAndSpace(&s, appContact):
					sw.Info.Contact.Email = s
				case tagTrimPrefixAndSpace(&s, appName):
					sw.Info.Contact.Name = s
				case tagTrimPrefixAndSpace(&s, appURL):
					if len(sw.Servers) == 0 {
						sw.Servers = append(sw.Servers, swaggerv3.Server{})
					}
					sw.Servers[0].URL = s
					// case tagTrimPrefixAndSpace(&s, appSchemes):
					// 	sw.Schemes = strings.Split(s, ",")
					// case tagTrimPrefixAndSpace(&s, appHost):
					// 	sw.Host = s
					// case tagTrimPrefixAndSpace(&s, appBasePath):
					// 	sw.BasePath = s
					// case tagTrimPrefixAndSpace(&s, appConsumes):
					// 	sw.Consumes = contentTypeByDoc(s)
					// case tagTrimPrefixAndSpace(&s, appProduces):
					// 	sw.Produces = contentTypeByDoc(s)
				}
			}
		}
	}

	// Analyse controller package
	// like:
	// swagger.go
	// import (
	//     _ "path/to/ctrl1"
	//     _ "path/to/ctrl2"
	//     _ "path/to/ctrl3"
	// )
	// // @APIVersion xxx
	// // @....
	for _, im := range f.Imports {
		importPath := strings.Trim(im.Path.Value, "\"")
		p, err := newResoucre(importPath, true)
		if err != nil {
			return err
		}
		if err = p.run(sw); err != nil {
			return err
		}
	}
	return nil
}
