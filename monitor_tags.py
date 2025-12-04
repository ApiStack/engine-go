import time
import urllib.request
import json
import sys

def monitor():
    url = "http://localhost:8080/api/tags"
    print(f"Monitoring tags at {url}...")
    
    last_count = -1
    
    try:
        while True:
            try:
                with urllib.request.urlopen(url, timeout=1) as response:
                    if response.status == 200:
                        data = response.read()
                        tags = json.loads(data)
                        count = len(tags)
                        
                        # Always print if count changed or we have tags (to show progress)
                        if count != last_count or count > 0:
                            print(f"Active Tags: {count}")
                            last_count = count
                            
                            for tag in tags:
                                # tag structure: {id, ts, x, y, z, layer, flag}
                                flag = tag.get('flag', 0)
                                print(f"Tag {tag['id']:X}: ({tag['x']:.2f}, {tag['y']:.2f}, {tag['z']:.2f}) Flag={flag}")
                            print("-" * 20)
                    else:
                        print(f"Error: {response.status}")
            except urllib.error.URLError:
                print("Connection refused. Server might be starting...")
            except Exception as e:
                print(f"Error: {e}")
            
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nStopping monitor.")

if __name__ == "__main__":
    monitor()