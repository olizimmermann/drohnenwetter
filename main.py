import requests
from bs4 import BeautifulSoup
import re


headers = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"}

def get_all_airports():
#     GET /metar-taf/deutschland.php HTTP/2
# Host: de.allmetsat.com
    url = "https://de.allmetsat.com/metar-taf/deutschland.php"
    response = requests.get(url, headers=headers)
    if response.status_code == 200:
        soup = BeautifulSoup(response.text, 'html.parser')
        airports = {}
        for option in soup.find_all('option'):
            value = option.get('value')
            value = value[-4:]
            text = option.text.strip()
            if value and value.isalpha() and len(value) == 4:  # Simple check for ICAO codes
                airports[text] = value
        return airports
    else:
        print(f"Failed to retrieve data. Status code: {response.status_code}")
        return []

def check_if_city_has_airport(city_name):
    airports = get_all_airports()
    for airport_name, airport_code in airports.items():
        if city_name.lower() in airport_name.lower():
            return airport_code
    return None


def return_metar_for_airport(airport_code):
    url = f"https://de.allmetsat.com/metar-taf/deutschland.php?icao={airport_code}"
    headers = {
        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
        "Accept-Language": "en-US,en;q=0.9",
        "Referer": "https://de.allmetsat.com/metar-taf/deutschland.php",
    }
    response = requests.get(url, headers=headers)
    if response.status_code == 200:
        soup = BeautifulSoup(response.text, 'html.parser')
        for b_tag in soup.find_all('b'):
            if b_tag.text == 'METAR:':
                return b_tag.parent.text.strip().replace('METAR:', '').strip()
    return None

def return_taf_for_airport(airport_code):
    url = f"https://de.allmetsat.com/metar-taf/deutschland.php?icao={airport_code}"
    headers = {
        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
        "Accept-Language": "en-US,en;q=0.9",
        "Referer": "https://de.allmetsat.com/metar-taf/deutschland.php",
    }
    response = requests.get(url, headers=headers)
    if response.status_code == 200:
        soup = BeautifulSoup(response.text, 'html.parser')
        for b_tag in soup.find_all('b'):
            if b_tag.text == 'TAF:':
                return b_tag.parent.text.strip().replace('TAF:', '').strip()
    return None

if __name__ == "__main__":
    city = input("Enter a city name: ")
    airport_code = check_if_city_has_airport(city)
    if airport_code:
        print(f"Airport code for {city}: {airport_code}")
        metar = return_metar_for_airport(airport_code)
        taf = return_taf_for_airport(airport_code)
        print(f"METAR for {airport_code}: {metar}")
        print(f"TAF for {airport_code}: {taf}")
        # METAR for EDDS: EDDS 221250Z AUTO 25011KT 220V290 9999 VCSH FEW031 SCT047 FEW///TCU 11/07 Q1021 NOSIG
        # TAF for EDDS: EDDS 221100Z 2212/2312 22008KT 9999 BKN025 PROB30 TEMPO 2217/2304 RA PROB30 TEMPO 2310/2312 24015G25KT
        regex = r"BKN(\d{3})"
        result = re.search(regex, taf)
        if result:
            cloud_base_ft = result.group(1)
        else:
            cloud_base_ft = 0
        
        cloud_base = int(cloud_base_ft) * 100
        print(f"Cloud base in feet: {cloud_base}ft")
        cloud_base_m = cloud_base * 0.3048
        print(f"Cloud base in meters: {cloud_base_m}m")

    else:
        print(f"No airport found for city: {city}")
        