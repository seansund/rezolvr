// © Copyright IBM Corporation 2020. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"rezolvr/model"
	"strings"
	"text/template"
)

type rezolvrDriver struct{}

// PrintMessage prints... a message
func (rd rezolvrDriver) PrintMessage() {
	fmt.Println("Hello from the Kubernetes plugin")
}

type providesTemplate struct {
	name     string
	Type     string
	contents string
}

func (rd rezolvrDriver) loadTemplate(baseDir string, templateName string) (providesTemplate, error) {
	templateType := strings.Split(templateName, ".")[0]
	content, err := ioutil.ReadFile(baseDir + "templates/" + templateName + ".template")
	if err != nil {
		msg := fmt.Sprintf("Error loading template: %v. Error: %v", templateName, err)
		log.Println(msg)
	}
	curTemplate := providesTemplate{Type: templateType, contents: string(content)}
	return curTemplate, nil
}

func (rd rezolvrDriver) populateTemplate(templateSource string, curProvides *model.Resource, c *model.Component, platformSettings map[string]*model.Platform) string {

	// Reconcile default and resource-specific platform settings
	defaultPlatformSettings := platformSettings["default"]
	resourcePlatformSettings := platformSettings[curProvides.Name]
	resolvedPlatformSettings := model.Platform{}
	resolvedPlatformSettings.Params = make(map[string]*model.Param)
	for k, v := range defaultPlatformSettings.Params {
		resolvedPlatformSettings.Params[k] = v
	}
	if resourcePlatformSettings != nil {
		for k, v := range resourcePlatformSettings.Params {
			resolvedPlatformSettings.Params[k] = v
		}
	}

	// The following two variables are made available to the eval() method
	data := map[string]interface{}{
		"Platform":      resolvedPlatformSettings.Params,
		"Provides":      curProvides,
		"ProvideParams": curProvides.Params,
		"Uses":          c.Uses,
		"Component":     c,
	}

	t := template.Must(template.New("").Parse(templateSource))
	buf := &bytes.Buffer{}
	err := t.Execute(buf, data)
	if err != nil {
		msg := fmt.Sprintf("Error resolving a Kubernetes template: %v", err)
		log.Printf(msg)
		return ""
	}
	stringVal := buf.String()
	return stringVal
}

func (rd rezolvrDriver) transformProvidedResource(r *model.Component, pluginDir string, state *model.State, platformSettings map[string]*model.Platform) []providesTemplate {
	results := make([]providesTemplate, 0)
	for _, curProvides := range r.Provides {
		// Some resources do not generate output, because they're external resources. Check the platform settings for this resource
		var isExternalParam *model.Param = nil
		resourcePlatformSettings := platformSettings[curProvides.Name]
		if resourcePlatformSettings != nil {
			isExternalParam = resourcePlatformSettings.Params["isExternal"]
		}
		if isExternalParam != nil && isExternalParam.Value == "true" {
			log.Printf("Based on the platform settings, a template will not be generated for: %s. (isExternal=true)\n", curProvides.Name)
		} else {
			template, err := rd.loadTemplate(pluginDir, curProvides.Type)
			if err != nil {
				log.Printf("WARNING: Template not found: %v\n", curProvides.Type)
			} else if len(template.contents) < 5 {
				log.Printf("Empty template found. Output will be skipped for: %v\n", curProvides.Type)
			} else {
				filledInTemplate := rd.populateTemplate(template.contents, curProvides, r, platformSettings)
				results = append(results, providesTemplate{name: curProvides.Name, Type: template.Type, contents: filledInTemplate})
			}
		}
	}
	return results
}

func (rd rezolvrDriver) TransformComponents(updatedComponents map[string]*model.Component, state *model.State, pluginDir string, outputDir string, platformSettings map[string]*model.Platform) {
	if updatedComponents == nil {
		fmt.Println("No components / resources to transform")
		return
	}

	// This transformer regenerates all components within the state. However,
	// the updated components should take precedence, obviously. So, create a new map
	allComponents := make(map[string]*model.Component)
	for k, v := range state.Components {
		allComponents[k] = v
	}
	for k, v := range updatedComponents {
		allComponents[k] = v
	}

	// For now, all components / resources should be regenerated
	allServices := make(map[string]string)

	for _, curComponent := range allComponents {
		transformed := rd.transformProvidedResource(curComponent, pluginDir, state, platformSettings)
		for _, curProvides := range transformed {
			allServices[curProvides.name] = curProvides.contents
		}
	}
	// Write the contents to the OS
	if len(allServices) > 0 {
		err := rd.saveAsYaml(outputDir, allServices)
		if err != nil {
			fmt.Printf("Error encountered saving YAML file: %v", err)
		}
	} else {
		log.Println("No services were generated. Skipping the generation of Kubernetes files...")
	}
}

func (rd rezolvrDriver) saveAsYaml(outputDir string, services map[string]string) error {
	for k, v := range services {
		var str strings.Builder
		str.WriteString(v)
		b := []byte(str.String())
		fileName := outputDir + k + ".yaml"
		log.Printf("Writing file: %v\n", fileName)
		err := ioutil.WriteFile(fileName, b, 0644)
		if err != nil {
			msg := fmt.Sprintf("Error writing file: %v", err)
			log.Println(msg)
		}
	}
	return nil
}

// RezolvrDriver is the entry point for this plugin
var RezolvrDriver rezolvrDriver

func main() {
	fmt.Println("This is a plugin.")
}
