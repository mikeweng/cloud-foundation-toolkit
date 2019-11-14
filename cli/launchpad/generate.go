// Package launchpad file generate.go contains all output generation logic.
//
// A component is a set of related scripts that generally resides under the same
// folder beneath outputDirectory root. A functionality is a particular action
// that can be applied to a component to achieve some purpose.
//
// Output generation depends on evaluated gState, and looping through components
// in specified order to apply functionality in sequence to generate output
// based on defined outputFlavor.
package launchpad

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
)

// component interface allows implementer to be processed by general processing loop in generateOutput.
type component interface {
	componentName() string
}

// generateOutput cycles through components and applies functionality in sequence.
func generateOutput() {
	activeComponents := []component{
		newOutputDirectory(), // Create Top Level Output Directory for all Launchpad configs
		newFolders(),         // GCP Folder Generation
		newProjects(),        // GCP Project Generation
	}
	activeComponents = append(activeComponents, newProjectTmpls()...)

	for _, c := range activeComponents { // Apply Functionality to each component
		withDirectory(c)
		withFiles(c)
	}

	if gState.outputFlavor == outTf { // re-indent with terraform fmt
		if _, err := exec.Command("terraform", "fmt", "-recursive", gState.outputDirectory).Output(); err != nil {
			// Only warning user since output terraform files are technically able to execute, just not indented properly
			// TODO consider warning terraform version lower then 0.12
			log.Println("Failed to format terraform output")
		}
	}
}

// ==== Core Components ===

// outputDirectory serves as the top level output directory.
type outputDirectory struct{}

// directoryProperty specified output root's location and backup property.
func (l *outputDirectory) directoryProperty() *directoryProperty {
	return newDirectoryProperty(
		gState.outputDirectory,
		directoryPropertyBackup(false))
}
func newOutputDirectory() *outputDirectory       { return &outputDirectory{} }
func (l *outputDirectory) componentName() string { return "outputDirectory" }

// ==== Components ====
// folders component allows sub-directory generation under outputDirectory for GCP Folder related code.
type folders struct {
	YAMLs       map[string]*folderSpecYAML
	dirname     string
	dirProperty *directoryProperty
}

func newFolders() *folders               { return &gState.evaluated.folders }
func (f *folders) componentName() string { return "folders" }
func (f *folders) directoryProperty() *directoryProperty {
	if len(f.YAMLs) == 0 { // No need to create if no folders being generated
		return nil
	}
	if f.dirProperty == nil {
		f.dirProperty = newDirectoryProperty(f.componentName(), directoryPropertyBackup(false), directoryPropertyDirname(gState.outputDirectory))
	}
	return f.dirProperty
}
func (f *folders) files() (fs []file) {
	if len(f.YAMLs) == 0 {
		return
	}
	dir := f.dirProperty.path()
	switch gState.outputFlavor {
	case outTf:
		var outputCons, varCons []tfConstruct
		mainCons := []tfConstruct{newTfTerraform(tfTerraformVer), newTfGoogleProvider()}
		for _, y := range f.YAMLs {
			mainCons = append(mainCons, newTfGoogleFolder(y.Id, y.DisplayName, &y.ParentRef))
			outputCons = append(outputCons, newTfOutput(y.Id, fmt.Sprintf("google_folder.%s.name", y.Id)))
		}
		varCons = append(
			varCons,
			newTfVariable("organization_id", "GCP Organization ID", gState.evaluated.orgId),
			newTfVariable("credentials_file_path", "Service account key path", "credentials.json"),
		)

		return []file{
			newTfFile("main", dir, mainCons),
			newTfFile("output", dir, outputCons),
			newTfFile("variables", dir, varCons),
		}
	default:
		panic(errors.New("output format not yet implemented"))
	}
}

// projects component allows GCP Project generation under outputDirectory.
type projects struct {
	YAMLs       map[string]*projectSpecYAML
	dirname     string
	dirProperty *directoryProperty
}

func newProjects() *projects              { return &gState.evaluated.projects }
func (p *projects) componentName() string { return "projects" }
func (p *projects) directoryProperty() *directoryProperty {
	if len(p.YAMLs) == 0 {
		return nil
	}
	if p.dirProperty == nil {
		p.dirProperty = newDirectoryProperty(p.componentName(), directoryPropertyBackup(false), directoryPropertyDirname(gState.outputDirectory))
	}
	return p.dirProperty
}
func (p *projects) files() (fs []file) {
	if len(p.YAMLs) == 0 {
		return
	}
	dir := p.dirProperty.path()
	switch gState.outputFlavor {
	case outTf:
		var outputCons, varCons []tfConstruct
		mainCons := []tfConstruct{newTfTerraform(tfTerraformVer), newTfGoogleProvider()}
		for _, p := range p.YAMLs {
			mainCons = append(mainCons, newTfGoogleProject(p))
			outputCons = append(outputCons, newTfOutput(p.Id, fmt.Sprintf("project_%s", p.Id)))
		}
		varCons = append(
			varCons,
			newTfVariable("organization_id", "GCP Organization ID", gState.evaluated.orgId),
			newTfVariable("credentials_file_path", "Service account key path", "credentials.json"),
		)

		return []file{
			newTfFile("main", dir, mainCons),
			newTfFile("output", dir, outputCons),
			newTfFile("variables", dir, varCons),
		}
	default:
		panic(errors.New("output format not yet implemented"))
	}
}

// projectTmpls component is a single GCP Project Template generation within projects.
//
// projectTmpls is a wrapper holder for slice of projectTmpl, with each projectTmpl requiring a
// dedicated directory and sets of files.
//type projectTmpls struct {
//	YAMLs       map[string]*projectSpecYAML
//}

type projectTmpl struct {
	YAML        *projectSpecYAML
	dirname     string
	dirProperty *directoryProperty
}

func newProjectTmpls() []component {
	var buff []component
	for _, y := range gState.evaluated.projectTmplYAMLs {
		dirname := fmt.Sprintf("%s/templates/%s", gState.evaluated.projects.componentName(), y.Id)
		buff = append(buff, &projectTmpl{
			YAML:    y,
			dirname: dirname,
			dirProperty: newDirectoryProperty(
				dirname, directoryPropertyBackup(false),
				directoryPropertyDirname(gState.outputDirectory)),
		})
	}
	return buff
}
func (p *projectTmpl) componentName() string                 { return p.dirname }
func (p *projectTmpl) directoryProperty() *directoryProperty { return p.dirProperty }
func (p *projectTmpl) files() (fs []file) {
	dir := p.dirProperty.path()
	switch gState.outputFlavor {
	case outTf:
		mainCons := []tfConstruct{newTfGoogleProject(p.YAML)}
		varCons := []tfConstruct{
			newTfVariable("organization_id", "GCP Organization ID", gState.evaluated.orgId),
			newTfVariable("billing_account", "GCP Billing Account", p.YAML.BillingAccount),
		}
		return []file{
			newTfFile("main", dir, mainCons),
			newTfFile("variables", dir, varCons),
		}
	default:
		panic(errors.New("output format not yet implemented"))
	}
}
