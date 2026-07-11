package inventorysvc

import "testing"

func TestNormalizeItemsAggregatesAndSorts(t *testing.T) {
	items, err := normalizeItems([]ItemRequest{
		{ProductID: 9, Quantity: 2},
		{ProductID: 3, Quantity: 1},
		{ProductID: 9, Quantity: 4},
	})
	if err != nil {
		t.Fatalf("normalize items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ProductID != 3 || items[0].Quantity != 1 {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[1].ProductID != 9 || items[1].Quantity != 6 {
		t.Fatalf("unexpected second item: %+v", items[1])
	}
}

func TestNormalizeItemsRejectsInvalidQuantity(t *testing.T) {
	if _, err := normalizeItems([]ItemRequest{{ProductID: 1, Quantity: 0}}); err == nil {
		t.Fatal("expected invalid quantity error")
	}
}
