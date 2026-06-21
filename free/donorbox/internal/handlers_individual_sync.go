package internal

import (
	"context"
)

// --- Individual sync functions -----------------------------------------------

func syncCampaigns(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllCampaigns()
	if err != nil {
		return 0, err
	}

	records := make([]Campaign, 0, len(items))
	for _, c := range items {
		records = append(records, Campaign{
			ID:             c.ID,
			Name:           strPtr(c.Name),
			Slug:           strPtr(c.Slug),
			Currency:       defaultStr(c.Currency, "USD"),
			GoalAmount:     parseFloatPtr(c.GoalAmount),
			TotalRaised:    parseFloat(c.TotalRaised),
			DonationsCount: c.DonationsCount,
			IsActive:       c.IsActive,
			CreatedAt:      parseTimePtr(c.CreatedAt),
			UpdatedAt:      parseTimePtr(c.UpdatedAt),
		})
	}
	return db.UpsertCampaigns(ctx, records)
}

func syncDonors(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllDonors()
	if err != nil {
		return 0, err
	}

	records := make([]Donor, 0, len(items))
	for _, d := range items {
		records = append(records, Donor{
			ID:             d.ID,
			FirstName:      strPtrOrNil(d.FirstName),
			LastName:       strPtrOrNil(d.LastName),
			Email:          strPtrOrNil(d.Email),
			Phone:          strPtrOrNil(d.Phone),
			Address:        strPtrOrNil(d.Address),
			City:           strPtrOrNil(d.City),
			State:          strPtrOrNil(d.State),
			ZipCode:        strPtrOrNil(d.ZipCode),
			Country:        strPtrOrNil(d.Country),
			Employer:       strPtrOrNil(d.Employer),
			DonationsCount: d.DonationsCount,
			LastDonationAt: parseTimePtr(d.LastDonationAt),
			Total:          parseFloat(d.Total),
			CreatedAt:      parseTimePtr(d.CreatedAt),
			UpdatedAt:      parseTimePtr(d.UpdatedAt),
		})
	}
	return db.UpsertDonors(ctx, records)
}

func syncDonations(ctx context.Context, db *DB, client *DonorboxClient, dateFrom string) (int, error) {
	items, err := client.ListAllDonations(dateFrom, "")
	if err != nil {
		return 0, err
	}

	records := make([]Donation, 0, len(items))
	for _, d := range items {
		records = append(records, mapAPIDonation(d))
	}
	return db.UpsertDonations(ctx, records)
}

func syncPlans(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllPlans()
	if err != nil {
		return 0, err
	}

	records := make([]Plan, 0, len(items))
	for _, p := range items {
		records = append(records, Plan{
			ID:               p.ID,
			CampaignID:       intPtrNonZero(p.Campaign.ID),
			CampaignName:     strPtrOrNil(p.Campaign.Name),
			DonorID:          intPtrNonZero(p.Donor.ID),
			DonorEmail:       strPtrOrNil(p.Donor.Email),
			Type:             strPtrOrNil(p.Type),
			Amount:           parseFloat(p.Amount),
			Currency:         defaultStr(p.Currency, "USD"),
			Status:           strPtrOrNil(p.Status),
			StartedAt:        parseTimePtr(p.StartedAt),
			LastDonationDate: parseTimePtr(p.LastDonationDate),
			NextDonationDate: parseTimePtr(p.NextDonationDate),
			CreatedAt:        parseTimePtr(p.CreatedAt),
			UpdatedAt:        parseTimePtr(p.UpdatedAt),
		})
	}
	return db.UpsertPlans(ctx, records)
}

func syncEvents(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllEvents()
	if err != nil {
		return 0, err
	}

	records := make([]Event, 0, len(items))
	for _, e := range items {
		records = append(records, Event{
			ID:           e.ID,
			Name:         strPtrOrNil(e.Name),
			Slug:         strPtrOrNil(e.Slug),
			Description:  strPtrOrNil(e.Description),
			StartDate:    parseTimePtr(e.StartDate),
			EndDate:      parseTimePtr(e.EndDate),
			Timezone:     strPtrOrNil(e.Timezone),
			VenueName:    strPtrOrNil(e.VenueName),
			Address:      strPtrOrNil(e.Address),
			City:         strPtrOrNil(e.City),
			State:        strPtrOrNil(e.State),
			Country:      strPtrOrNil(e.Country),
			ZipCode:      strPtrOrNil(e.ZipCode),
			Currency:     defaultStr(e.Currency, "USD"),
			TicketsCount: e.TicketsCount,
			IsActive:     e.IsActive,
			CreatedAt:    parseTimePtr(e.CreatedAt),
			UpdatedAt:    parseTimePtr(e.UpdatedAt),
		})
	}
	return db.UpsertEvents(ctx, records)
}

func syncTickets(ctx context.Context, db *DB, client *DonorboxClient) (int, error) {
	items, err := client.ListAllTickets()
	if err != nil {
		return 0, err
	}

	records := make([]Ticket, 0, len(items))
	for _, t := range items {
		records = append(records, Ticket{
			ID:         t.ID,
			EventID:    intPtrNonZero(t.Event.ID),
			EventName:  strPtrOrNil(t.Event.Name),
			DonorID:    intPtrNonZero(t.Donor.ID),
			DonorEmail: strPtrOrNil(t.Donor.Email),
			TicketType: strPtrOrNil(t.TicketType),
			Quantity:   t.Quantity,
			Amount:     parseFloat(t.Amount),
			Currency:   defaultStr(t.Currency, "USD"),
			Status:     strPtrOrNil(t.Status),
			CreatedAt:  parseTimePtr(t.CreatedAt),
			UpdatedAt:  parseTimePtr(t.UpdatedAt),
		})
	}
	return db.UpsertTickets(ctx, records)
}

