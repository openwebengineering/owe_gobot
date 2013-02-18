// Steve Phillips / elimisteve
// 2011.05.27

package redmine

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	REDMINE_URL      = os.Getenv("REDMINE_URL")
	REDMINE_JSON_URL = REDMINE_URL + "/issues.json"
	REDMINE_API_KEY  = os.Getenv("REDMINE_API_KEY")
)

type Ticket struct {
	Project,
	TicketType,
	Subject,
	Description,
	Assignee     string
}

// Maps stated assignee's name to assigned_to_id
var assigneeToID = map[string]string {
	"aj":    "15876",
	"steve": "15872",
}

// Maps ticketType to tracker_id
var trackerID = map[string]string {
	"bug":     "1",
	"feature": "2",
}

func ParseTicket(msg string) *Ticket {
	ticketInfo := strings.SplitN(msg, " ", 3)
	if len(ticketInfo) == 3 {
		// Ignore leading '!' and trailing ' ' of command
		ticketType := strings.Trim(ticketInfo[0][1:], ` `)
		if ticketType != "bug" {
			ticketType = "feature"
		}
		project, subject := ticketInfo[1], ticketInfo[2]
		description, assignee := "", ""
		// If text after project_name contains a ';', the
		// user added a ticket description. Parse it out.
		if strings.Contains(subject, ";") {
			subDesc := strings.SplitN(subject, ";", 2)
			subject = strings.Trim(subDesc[0], ` `)
			description = strings.Trim(subDesc[1], ` `)
		}
		if strings.Contains(description, ";") {
			descAssignee := strings.Split(description, ";")
			description = strings.Trim(descAssignee[0], ` `)
			assignee = strings.Trim(descAssignee[1], ` `)
		}
		ticket := Ticket{
			Project: project,
			TicketType: ticketType,
			Subject: subject,
			Description: description,
			Assignee: assignee,
		}
		return &ticket
	}
	return nil
}

func CreateTicket(ticket *Ticket) (*http.Response, error) {
	// FIXME: Temporary;
	project := ticket.Project
	ticketType := ticket.TicketType
	subject := ticket.Subject
	description := ticket.Description
	assignee := ticket.Assignee

	// TODO: Marshal a struct instead of using strings
	// <GhettoSanitize>
	project = strings.Replace(project, `"`, `'`, -1)
	ticketType = strings.Replace(ticketType, `"`, `'`, -1)
	subject = strings.Replace(subject, `"`, `'`, -1)
	description = strings.Replace(description, `"`, `'`, -1)
	assignee = strings.Replace(assignee, `"`, `'`, -1)
	assignee = strings.ToLower(assignee)
	// </GhettoSanitize>

	json := fmt.Sprintf(`{"project_id": "%v", "issue": {"tracker_id": %v, "subject": "%v", "description": "%v", "assigned_to_id": %v}}`,
		project, trackerID[ticketType], subject, description, assigneeToID[assignee])
	reader := strings.NewReader(json)

	client := &http.Client{}
	req, err := http.NewRequest("POST", REDMINE_JSON_URL, reader)
	if err != nil {
		return nil, fmt.Errorf("Error POSTing JSON: %v", err)
	}
	req.Header.Add("X-Redmine-API-Key", REDMINE_API_KEY)
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("client.Do error: %v\n", err)
		return nil, err
	}
	return resp, nil
}
