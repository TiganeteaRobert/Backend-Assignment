package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
)

// App represents the server's internal state.
// It holds configuration about providers and content
type App struct {
	ContentClients map[Provider]Client
	Config         ContentMix
}

// Response represents a request response, wrapping the received content and error
type Response struct {
	Content []*Article
	Error   error
}

// getURLParams retrieves the URL params (count and offset)
func getURLParams(r *http.Request) (count, offset int, err error) {
	count, err = strconv.Atoi(r.URL.Query().Get("count"))
	if count < 0 {
		err = errors.New("invalid count")
		count = 0
	}
	offset, err = strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		err = errors.New("invalid offset")
		offset = 0
	}
	return
}

// ServeHTTP : main HTTP Handler for GET requests on server
// Concurrently executes getting content from providers, in manner detailed in README.md

// ServeHTTP handles HTTP requests on the server concurrently
func (a App) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var (
		// total number of articles retrieved so far
		articleCount = 0
		// used for handling adding to array in response
		addComma = false
	)

	// close array in response after everything is wrapped up
	defer w.Write([]byte("]"))

	// retrieve the query params
	count, offset, err := getURLParams(req)
	if err != nil {
		// log any error regarding URL params
		// if error appears count and offset are defaulted to 0
		log.Println(err)
	}

	// slice of channels, one for each content provider in configuration
	// using this, we can concurrently get content from each provider and get the data back in a specific order
	providerChans := make([]chan Response, len(a.Config))
	for i := offset; i < len(a.Config); i++ {
		providerChans[i] = make(chan Response)
	}

	// prepare response headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Status", "200")
	// prepare the response to represent an array by adding a square bracket
	w.Write([]byte("["))

	// loop until we have retrieved the desired number of articles.
	// each loop retrieves a new batch of len(a.Config) articles
	for articleCount < count {
		err := a.retrieveBatch(&addComma, offset, count, &articleCount, providerChans, w)
		if err != nil {
			return
		}
	}

	// leaving the garbage collector to close all channels to prevent a data race for channels when running
	// multiple tests which can happen with manual closing with a WaitGroup
	return
}

// marshalAndWrite handles marshaling a list of articles and adding it to the response
func marshalAndWrite(val []*Article, w *http.ResponseWriter) {
	// marshal articles
	m, mErr := json.Marshal(val[0])
	if mErr != nil {
		log.Println(mErr)
	}
	// write articles to response
	_, err := (*w).Write(m)
	if err != nil {
		log.Println(err)
	}
}

// retrieveBatch retrieves a batch of articles from each provider and adds it to the response
func (a App) retrieveBatch(addComma *bool, offset, count int, batchCount *int, providerChans []chan Response, w http.ResponseWriter) error {
	// initialise required vars
	var (
		idx = 0
	)
	// go through the providers and start a goroutine for each of them, checking that we have not
	// reached the desired number of responses yet. each goroutine will store the retrieved articles
	// in it's provider's channel.
	for idx = offset; idx < len(a.Config); idx++ {
		// start a goroutine for each provider in order to retrieve content and store it
		// in the provider's respective channel
		go a.retrieveContent(idx, providerChans[idx])
	}

	// iterate again from offset to the config length in order to retrieve values from the provider channels
	for idx = offset; idx < len(a.Config); idx++ {
		val := <-providerChans[idx]
		// break the loop by returning early if an error is found. this ensures that if an error is met
		// we will return the content retrieved until the point of error
		if val.Error != nil {
			log.Println(val.Error)
			return errors.New("value error")
		}

		// this ensures that we are correctly creating our array for the response
		// by attaching a comma after each element except the first one
		if *addComma {
			w.Write([]byte(","))
		}
		// start adding commas after first element
		*addComma = true

		// add the content to the response
		marshalAndWrite(val.Content, &w)
		*batchCount++
		if *batchCount == count {
			return nil
		}
	}

	// offset can be reset to 0 after the first loop
	offset = 0
	return nil
}

// retrieveContent calls the provider's GetContent method and retrieves the content in the provider's channel
func (a App) retrieveContent(idx int, ch chan Response) {
	// try to get the content using the provider GetContent method
	content, err := a.ContentClients[a.Config[idx].Type].GetContent("Test", 1)
	if err != nil {
		// if first get failed, attempt to get from fallback
		hasFallBack := a.Config[idx].Fallback
		if hasFallBack != nil {
			fallbackContent, fallbackErr := a.ContentClients[*a.Config[idx].Fallback].GetContent("Test", 1)
			// send content and potential error to channel
			ch <- Response{
				Content: fallbackContent,
				Error:   fallbackErr,
			}
			return
		}
		ch <- Response{
			Content: nil,
			Error:   errors.New("no fallback found"),
		}
		return
	}
	// send content to channel in case when the first GetContent was successful
	ch <- Response{
		Content: content,
		Error:   nil,
	}
	return
}
