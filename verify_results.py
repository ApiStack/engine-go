import csv
import math
import sys

def analyze_csv(path):
    rows = []
    with open(path, 'r') as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append(row)
    
    if not rows:
        print("No data found")
        return

    xs = [float(r['fused_x_m']) for r in rows]
    ys = [float(r['fused_y_m']) for r in rows]
    
    min_x, max_x = min(xs), max(xs)
    min_y, max_y = min(ys), max(ys)
    
    print(f"Count: {len(rows)}")
    print(f"X Bounds: {min_x:.4f} to {max_x:.4f}")
    print(f"Y Bounds: {min_y:.4f} to {max_y:.4f}")
    
    # Check for outliers > 2000m
    outliers = [x for x in xs if abs(x) > 1000] + [y for y in ys if abs(y) > 1000]
    if outliers:
        print(f"FAIL: Found {len(outliers)} outlier coordinates > 1000m")
        sys.exit(1)
    
    # Check for clamping at -2000
    clamped = [x for x in xs if x == -2000.0] + [y for y in ys if y == -2000.0]
    if clamped:
        print(f"FAIL: Found {len(clamped)} coordinates clamped exactly at -2000.0")
        sys.exit(1)

    print("PASS: Coordinates are within reasonable bounds.")

if __name__ == "__main__":
    analyze_csv("engine-go/verify_fix.csv")
