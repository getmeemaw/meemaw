package config

import (
	"errors"
	"log"
	"os"
	"strconv"
)

func CheckRequiredEnvVars(requiredVars []string) error {
	for _, v := range requiredVars {
		if _, exists := os.LookupEnv(v); !exists {
			return errors.New("Required environment variable" + v + "not set")
		}
	}
	return nil
}

func GetEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		log.Printf("Invalid value for %s: %v, using default %d", key, err, defaultValue)
		return defaultValue
	}
	return value
}

func GetEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		log.Printf("Invalid value for %s: %v, using default %t", key, err, defaultValue)
		return defaultValue
	}
	return value
}
