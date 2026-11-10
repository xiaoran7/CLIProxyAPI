// 凭证时序分桶吞吐查询 —— 拆自 queries_upstream.go
package db

import (
	"context"
	"fmt"
)

// AuthBucketRow captures aggregated throughput for an auth_index in a 10-minute bucket.
type AuthBucketRow struct {
	AuthIndex string
	BucketID  int64
	Success   int64
	Failed    int64
}

// SelectRecentAuthBuckets returns 10-minute bucket aggregates from request_events for ts >= sinceTS.
func (d *DB) SelectRecentAuthBuckets(ctx context.Context, sinceTS int64) ([]AuthBucketRow, error) {
	query := `
SELECT 
  auth_index,
  (ts / 600) AS bucket_id,
  COALESCE(SUM(CASE WHEN status >= 200 AND status < 400 THEN 1 ELSE 0 END), 0) AS success,
  COALESCE(SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END), 0) AS failed
FROM request_events
WHERE ts >= ?
GROUP BY auth_index, bucket_id
HAVING auth_index != ''
`
	rows, err := d.QueryContext(ctx, query, sinceTS)
	if err != nil {
		return nil, fmt.Errorf("select recent auth buckets failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []AuthBucketRow
	for rows.Next() {
		var r AuthBucketRow
		if err := rows.Scan(&r.AuthIndex, &r.BucketID, &r.Success, &r.Failed); err != nil {
			return nil, fmt.Errorf("scan recent auth buckets failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}
