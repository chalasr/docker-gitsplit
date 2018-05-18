package gitsplit

import (
    "fmt"
    "io/ioutil"
    "gopkg.in/yaml.v2"
    "github.com/pkg/errors"
    "github.com/jderusse/gitsplit/utils"
)

type StringCollection []string

type Split struct {
    Prefixes StringCollection  `yaml:"prefix"`
    Targets  StringCollection  `yaml:"target"`
}

type Config struct {
    CacheUri   *GitUri    `yaml:"cache_dir"`
    ProjectUri *GitUri    `yaml:"project_dir"`
    Splits     []Split   `yaml:"splits"`
    Origins    []string  `yaml:"origins"`
}

func (s *GitUri) UnmarshalYAML(unmarshal func(interface{}) error) error {
    var raw interface{}
    if err := unmarshal(& raw); err != nil {
        return err
    }


    switch raw.(type){
        case string:
            *s = *ParseUri(raw.(string))
        default:
            return fmt.Errorf("expects a string")
    }

    return nil
}

func (s *StringCollection) UnmarshalYAML(unmarshal func(interface{}) error) error {
    var raw interface{}
    if err := unmarshal(& raw); err != nil {
        return err
    }

    switch raw.(type){
        case string:
            *s = []string{raw.(string)}
        case []string:
            *s = raw.([]string)
        default:
            return fmt.Errorf("expects a string or n array of strings")
    }

    return nil
}

func NewConfigFromFile(filePath string) (*Config, error) {
    config := &Config{}

    yamlFile, err := ioutil.ReadFile(utils.ResolvePath(filePath))
    if err != nil {
        return nil, errors.Wrap(err, "Fail to read config file")
    }

    err = yaml.Unmarshal(yamlFile, &config)
    if err != nil {
        return nil, errors.Wrap(err, "Fail to load config file")
    }

    if config.ProjectUri == nil {
        config.ProjectUri = ParseUri(".")
    }

    if len(config.Origins) == 0 {
        config.Origins =  []string{".*"}
    }

    return config, nil
}
