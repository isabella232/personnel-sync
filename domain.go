package personnel_sync

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strings"
)

const DefaultConfigFile = "./config.json"
const DestinationTypeGoogleGroups = "GoogleGroups"
const DestinationTypeWebHelpDesk = "WebHelpDesk"
const SourceTypeRestAPI = "RestAPI"

// LoadConfig looks for a config file if one is provided. Otherwise, it looks for
// a config file based on the CONFIG_PATH env var.  If that is not set, it gets
// the default config file ("./config.json").
func LoadConfig(configFile string) (AppConfig, error) {

	if configFile == "" {
		configFile = os.Getenv("CONFIG_PATH")
		if configFile == "" {
			configFile = DefaultConfigFile
		}
	}

	log.Printf("Using config file: %s\n", configFile)

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Printf("unable to application config file %s, error: %s\n", configFile, err.Error())
		return AppConfig{}, err
	}

	config := AppConfig{}
	err = json.Unmarshal(data, &config)
	if err != nil {
		log.Printf("unable to unmarshal application configuration file data, error: %s\n", err.Error())
		return AppConfig{}, err
	}

	log.Printf("Configuration loaded. Source type: %s, Destination type: %s\n", config.Source.Type, config.Destination.Type)
	log.Printf("%v Sync sets found:\n", len(config.SyncSets))

	for i, syncSet := range config.SyncSets {
		log.Printf("  %v) %s\n", i+1, syncSet.Name)
	}

	return config, nil
}

// RemapToDestinationAttributes returns a slice of Person instances that each have
// only the desired attributes based on the destination attribute keys.
// If a required attribute is missing for a Person, then their disableChanges
// value is set to true.
func RemapToDestinationAttributes(sourcePersons []Person, attributeMap []AttributeMap) ([]Person, error) {
	var peopleForDestination []Person

	for _, person := range sourcePersons {
		attrs := map[string]string{}

		// Build attrs with only attributes from destination map, disable changes on person missing a required attribute
		disableChanges := false
		for _, attrMap := range attributeMap {
			if value, ok := person.Attributes[attrMap.Source]; ok {
				attrs[attrMap.Destination] = value
			} else if attrMap.Required {
				jsonAttrs, _ := json.Marshal(attrs)
				log.Printf("user missing attribute %s. Rest of data: %s", attrMap.Source, jsonAttrs)
				disableChanges = true
			}
		}

		peopleForDestination = append(peopleForDestination, Person{
			CompareValue:   person.CompareValue,
			Attributes:     attrs,
			DisableChanges: disableChanges,
		})

	}

	return peopleForDestination, nil
}

// IsPersonInList returns true if the lower-case version of the compareValue matches
// any of the lower-case versions of the CompareValues of the Person instances.
func IsPersonInList(compareValue string, peopleList []Person) bool {
	lowerCompareValue := strings.ToLower(compareValue)

	for _, person := range peopleList {
		if strings.ToLower(person.CompareValue) == lowerCompareValue {
			return true
		}
	}

	return false
}

const PersonIsNotInList = int(0)
const PersonIsInList = int(1)
const PersonIsInListButDifferent = int(2)

// PersonStatusInList returns an integer that denotes whether a Person instance is included in a slice,
// and if so whether they have different attributes than expected.
// Note this is based on comparing the lower-case version of the compareValue with the
// lower-case versions of each Person's CompareValue.
func PersonStatusInList(compareValue string, attrs map[string]string, peopleList []Person) int {
	lowerCompareValue := strings.ToLower(compareValue)

	for _, person := range peopleList {
		if strings.ToLower(person.CompareValue) == lowerCompareValue {
			if !reflect.DeepEqual(attrs, person.Attributes) {
				log.Printf("Attributes not equal: %v, %v\n", attrs, person.Attributes)
				return PersonIsInListButDifferent
			}
			return PersonIsInList
		}
	}

	return PersonIsNotInList
}

// GenerateChangeSet builds the three slice attributes of a ChangeSet
// (Create, Update and Delete) based on whether they are in the slice
//  of destination Person instances.
// It skips all source Person instances that have DisableChanges set to true
func GenerateChangeSet(sourcePeople, destinationPeople []Person) ChangeSet {
	var changeSet ChangeSet

	// Find users who need to be created
	for _, sp := range sourcePeople {
		// If user was missing a required attribute, don't change their record
		if sp.DisableChanges {
			continue
		}

		personInDestinationStatus := PersonStatusInList(sp.CompareValue, sp.Attributes, destinationPeople)
		switch personInDestinationStatus {
		case PersonIsNotInList:
			changeSet.Create = append(changeSet.Create, sp)
		case PersonIsInListButDifferent:
			changeSet.Update = append(changeSet.Update, sp)
		}
	}

	// Find users who need to be deleted
	for _, dp := range destinationPeople {
		if !IsPersonInList(dp.CompareValue, sourcePeople) {
			changeSet.Delete = append(changeSet.Delete, dp)
		}
	}

	return changeSet
}

// SyncPeople calls a number of functions to do the following ...
//  - it gets the list of people from the source
//  - it remaps their attributes to match the keys used in the destination
//  - it gets the list of people from the destination
//  - it generates the lists of people to change, update and delete
//  - if dryRun is true, it prints those lists, but otherwise makes the associated changes
func SyncPeople(source Source, destination Destination, attributeMap []AttributeMap, dryRun bool) ChangeResults {
	sourcePeople, err := source.ListUsers()
	if err != nil {
		return ChangeResults{
			Errors: []string{err.Error()},
		}
	}
	log.Printf("    Found %v people in source", len(sourcePeople))

	// remap source people to destination attributes for comparison
	sourcePeople, err = RemapToDestinationAttributes(sourcePeople, attributeMap)
	if err != nil {
		return ChangeResults{
			Errors: []string{err.Error()},
		}
	}

	destinationPeople, err := destination.ListUsers()
	if err != nil {
		return ChangeResults{
			Errors: []string{err.Error()},
		}
	}
	log.Printf("    Found %v people in destination", len(destinationPeople))

	changeSet := GenerateChangeSet(sourcePeople, destinationPeople)

	// If in DryRun mode only print out ChangeSet plans and return mocked change results based on plans
	if dryRun {
		printChangeSet(changeSet)
		return ChangeResults{
			Created: uint64(len(changeSet.Create)),
			Updated: uint64(len(changeSet.Update)),
			Deleted: uint64(len(changeSet.Delete)),
		}
	}

	return destination.ApplyChangeSet(changeSet)
}

func printChangeSet(changeSet ChangeSet) {
	log.Printf("ChangeSet Plans: Create %v, Update %v, Delete %v\n", len(changeSet.Create), len(changeSet.Update), len(changeSet.Delete))

	log.Println("Users to be created...")
	for i, user := range changeSet.Create {
		log.Printf("  %v) %s", i+1, user.CompareValue)
	}

	log.Println("Users to be updated...")
	for i, user := range changeSet.Update {
		log.Printf("  %v) %s", i+1, user.CompareValue)
	}

	log.Println("Users to be deleted...")
	for i, user := range changeSet.Delete {
		log.Printf("  %v) %s", i+1, user.CompareValue)
	}
}

// This function will search element inside array with any type.
// Will return boolean and index for matched element.
// True and index more than 0 if element is exist.
// needle is element to search, haystack is slice of value to be search.
func InArray(needle interface{}, haystack interface{}) (exists bool, index int) {
	exists = false
	index = -1

	switch reflect.TypeOf(haystack).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(haystack)

		for i := 0; i < s.Len(); i++ {
			if reflect.DeepEqual(needle, s.Index(i).Interface()) == true {
				index = i
				exists = true
				return
			}
		}
	}

	return
}

type EmptyDestination struct{}

func (e *EmptyDestination) ForSet(syncSetJson json.RawMessage) error {
	return nil
}

func (e *EmptyDestination) ListUsers() ([]Person, error) {
	return []Person{}, nil
}

func (e *EmptyDestination) ApplyChangeSet(changes ChangeSet) ChangeResults {
	return ChangeResults{}
}

type EmptySource struct{}

func (e *EmptySource) ForSet(syncSetJson json.RawMessage) error {
	return nil
}

func (e *EmptySource) ListUsers() ([]Person, error) {
	return []Person{}, nil
}
