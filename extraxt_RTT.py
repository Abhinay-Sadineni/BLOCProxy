import requests

def fetch_tcp_stats(envoy_host='localhost', envoy_port=9901):
    url = f'http://{envoy_host}:{envoy_port}/stats'
    response = requests.get(url)
    return response.text

def parse_rtt_stats(stats_text):
    rtt_stats = {}
    for line in stats_text.splitlines():
        if 'tcp_stats' in line:
            key, value = line.split(': ')
            rtt_stats[key] = value
    return rtt_stats

# Fetch and parse the RTT statistics
stats_text = fetch_tcp_stats()
rtt_stats = parse_rtt_stats(stats_text)
print(rtt_stats)
