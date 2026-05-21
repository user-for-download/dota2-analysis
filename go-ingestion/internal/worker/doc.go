// Package worker contains the three pipeline stages:
//
//	fetcher  → pulls match IDs from the queue, fetches raw payloads
//	           from upstream via a proxy pool, and stores blobs.
//	parser   → reads blobs, decodes JSON, validates, and hands the
//	           model to the ingester.
//	ingester → writes validated match models to the primary store
//	           (Postgres), gated by a dedup set.
package worker
