package internal

import (
	"encoding/json"
	"time"
)

// -------------------------------------------------------------------------
// Conversion helpers: Shopify API types -> DB model types
// -------------------------------------------------------------------------

func convertProduct(sp *shopifyProduct) Product {
	p := Product{
		ShopifyID:   sp.ID,
		Title:       sp.Title,
		BodyHTML:     strPtr(sp.BodyHTML),
		Vendor:      strPtr(sp.Vendor),
		ProductType: strPtr(sp.ProductType),
		Handle:      strPtr(sp.Handle),
		Status:      sp.Status,
		Tags:        strPtr(sp.Tags),
		Images:      sp.Images,
		Options:     sp.Options,
	}
	if sp.PublishedAt != nil {
		if t, err := time.Parse(time.RFC3339, *sp.PublishedAt); err == nil {
			p.PublishedAt = &t
		}
	}
	return p
}

func convertVariant(sv *shopifyVariant) Variant {
	v := Variant{
		ShopifyID:         sv.ID,
		Title:             strPtr(sv.Title),
		Price:             strPtr(sv.Price),
		CompareAtPrice:    sv.CompareAtPrice,
		SKU:               strPtr(sv.SKU),
		Barcode:           sv.Barcode,
		Position:          sv.Position,
		InventoryQuantity: sv.InventoryQuantity,
		Weight:            &sv.Weight,
		WeightUnit:        strPtr(sv.WeightUnit),
		Option1:           sv.Option1,
		Option2:           sv.Option2,
		Option3:           sv.Option3,
	}
	if sv.InventoryItemID != 0 {
		v.InventoryItemID = &sv.InventoryItemID
	}
	return v
}

func convertCollection(sc *shopifyCollection) Collection {
	c := Collection{
		ShopifyID:      sc.ID,
		Title:          sc.Title,
		BodyHTML:        strPtr(sc.BodyHTML),
		Handle:         strPtr(sc.Handle),
		SortOrder:      strPtr(sc.SortOrder),
		CollectionType: strPtr(sc.CollectionType),
		Image:          sc.Image,
	}
	if sc.PublishedAt != nil {
		if t, err := time.Parse(time.RFC3339, *sc.PublishedAt); err == nil {
			c.PublishedAt = &t
		}
	}
	return c
}

func convertCustomer(sc *shopifyCustomer) Customer {
	return Customer{
		ShopifyID:        sc.ID,
		Email:            strPtr(sc.Email),
		FirstName:        strPtr(sc.FirstName),
		LastName:         strPtr(sc.LastName),
		Phone:            sc.Phone,
		OrdersCount:      sc.OrdersCount,
		TotalSpent:       strPtr(sc.TotalSpent),
		Currency:         strPtr(sc.Currency),
		Tags:             strPtr(sc.Tags),
		Addresses:        sc.Addresses,
		DefaultAddress:   sc.DefaultAddress,
		AcceptsMarketing: sc.AcceptsMarketing,
	}
}

func convertOrder(so *shopifyOrder) Order {
	o := Order{
		ShopifyID:         so.ID,
		Name:              so.Name,
		Email:             strPtr(so.Email),
		TotalPrice:        strPtr(so.TotalPrice),
		SubtotalPrice:     strPtr(so.SubtotalPrice),
		TotalTax:          strPtr(so.TotalTax),
		TotalDiscounts:    strPtr(so.TotalDiscounts),
		Currency:          so.Currency,
		FinancialStatus:   strPtr(so.FinancialStatus),
		FulfillmentStatus: so.FulfillmentStatus,
		CustomerID:        so.CustomerID,
		ShippingAddress:   so.ShippingAddress,
		BillingAddress:    so.BillingAddress,
		Note:              so.Note,
		Tags:              strPtr(so.Tags),
		Gateway:           strPtr(so.Gateway),
		Confirmed:         so.Confirmed,
		CancelReason:      so.CancelReason,
	}
	if so.LineItems != nil {
		lineItemsJSON, _ := json.Marshal(so.LineItems)
		o.LineItems = lineItemsJSON
	}
	if so.CancelledAt != nil {
		if t, err := time.Parse(time.RFC3339, *so.CancelledAt); err == nil {
			o.CancelledAt = &t
		}
	}
	if so.ClosedAt != nil {
		if t, err := time.Parse(time.RFC3339, *so.ClosedAt); err == nil {
			o.ClosedAt = &t
		}
	}
	if so.ProcessedAt != nil {
		if t, err := time.Parse(time.RFC3339, *so.ProcessedAt); err == nil {
			o.ProcessedAt = &t
		}
	}
	return o
}

func convertLineItem(li *shopifyLineItem) OrderItem {
	return OrderItem{
		ShopifyID:         li.ID,
		ProductID:         li.ProductID,
		VariantID:         li.VariantID,
		Title:             li.Title,
		Quantity:          li.Quantity,
		Price:             strPtr(li.Price),
		SKU:               strPtr(li.SKU),
		Vendor:            strPtr(li.Vendor),
		FulfillmentStatus: li.FulfillmentStatus,
	}
}

// strPtr returns a pointer to s, or nil if s is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

