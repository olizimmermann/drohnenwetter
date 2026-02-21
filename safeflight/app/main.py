import asyncio
import logging
import sys
import os
from os import getenv
import pytz
from typing import Any, Dict
import urllib.request
import json

import requests
from datetime import datetime, timedelta
import math
from fastapi import FastAPI, Request, Form, HTTPException, Depends, status
from fastapi.templating import Jinja2Templates
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse
from fastapi.responses import HTMLResponse
from fastapi.security import HTTPBasic, HTTPBasicCredentials
from typing import List
from passlib.context import CryptContext
from slowapi import Limiter
from slowapi.util import get_remote_address
from slowapi.errors import RateLimitExceeded


openweather_token = getenv("OPENWEATHER_TOKEN")
here_api_key = getenv("HERE_API_KEY")

app = FastAPI()
limiter = Limiter(key_func=get_remote_address)
templates = Jinja2Templates(directory="app/templates")
app.mount("/static", StaticFiles(directory="app/static"), name="static") # Serve static files

# Konfigurieren des Loggings
if os.path.exists('/var/log/safeflight') == True:
    logging.basicConfig(filename='/var/log/safeflight/log.txt', level=logging.INFO, format='%(asctime)s %(message)s')
else:
    logging.basicConfig(stream=sys.stdout, level=logging.INFO, format='%(asctime)s %(message)s')


# Initialize HTTPBasic for basic authentication
security = HTTPBasic()

# Example hashed password (hash using a tool like Passlib)
pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")
users_db = {
    "username": {
        "username": os.getenv("BASIC_AUTH_USERNAME"),
        "hashed_password": pwd_context.hash(os.getenv("BASIC_AUTH_PASSWORD")),
    }
}
def verify_password(plain_password, hashed_password):
    return pwd_context.verify(plain_password, hashed_password)

def authenticate_user(username: str, password: str):
    user = users_db.get("username")
    for user in users_db.values():
        if user["username"] == username:
            if not user or not verify_password(password, user["hashed_password"]):
                return False
            return user
            

# Dependency for basic auth
def basic_auth(credentials: HTTPBasicCredentials = Depends(security)):
    user = authenticate_user(credentials.username, credentials.password)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid authentication credentials",
            headers={"WWW-Authenticate": "Basic"},
        )
    return user


async def __addstatus__(url,status):
    if status == 'def':
        url = url + '&status=def'
    return url 

async def get_getKpindex(starttime, endtime, index, status='all'):
    """
    ---------------------------------------------------------------------------------
    download 'Kp', 'ap', 'Ap', 'Cp', 'C9', 'Hp30', 'Hp60', 'ap30', 'ap60', 'SN', 'Fobs' or 'Fadj' index data from kp.gfz-potsdam.de
    date format for starttime and endtime is 'yyyy-mm-dd' or 'yyyy-mm-ddTHH:MM:SSZ'
    optional 'def' parameter to get only definitve values (only available for 'Kp', 'ap', 'Ap', 'Cp', 'C9', 'SN')
    Hpo index and Fobs/Fadj does not have the status info
    example: (time, index, status) = getKpindex('2021-09-29', '2021-10-01','Ap','def')
    example: (time, index, status) = getKpindex('2021-09-29T12:00:00Z', '2021-10-01T12:00:00Z','Kp')
    ---------------------------------------------------------------------------------
    """
    result_t=0; result_index=0; result_s=0

    if len(starttime) == 10 and len(endtime) == 10:
        starttime = starttime + 'T00:00:00Z'
        endtime = endtime + 'T23:59:00Z'

    try:
        d1 = datetime.strptime(starttime, '%Y-%m-%dT%H:%M:%SZ')
        d2 = datetime.strptime(endtime, '%Y-%m-%dT%H:%M:%SZ')

        time_string = "start=" + d1.strftime('%Y-%m-%dT%H:%M:%SZ') + "&end=" + d2.strftime('%Y-%m-%dT%H:%M:%SZ')
        url = 'https://kp.gfz-potsdam.de/app/json/?' + time_string  + "&index=" + index
        if index not in ['Hp30', 'Hp60', 'ap30', 'ap60', 'Fobs', 'Fadj']:
            url = await __addstatus__(url, status)

        ret = requests.get(url, timeout=3)
        if ret.status_code != 200:
            return 'N/A'
        else:
            kp_index = ret.json()
            kp_index = kp_index['Kp'][-1]
            return kp_index
    except Exception as e:
        logging.error(f"Error fetching KP index: {e}")
        return 'N/A'



async def get_hereapi_geocode(q):
    url = "https://geocode.search.hereapi.com/v1/geocode"
    params = {
        'q': q,
        'in': 'countryCode:DEU',
        'limit': 1,
        'lang': 'en',
        'apiKey': here_api_key
    }
    headers = {
        'Host': 'geocode.search.hereapi.com',
        'Sec-Ch-Ua-Platform': 'macOS',
        'Accept-Language': 'en-US,en;q=0.9',
        'Sec-Ch-Ua': '"Not?A_Brand";v="99", "Chromium";v="130"',
        'User-Agent': 'safeflight',
        'Sec-Ch-Ua-Mobile': '?0',
        'Accept': '*/*',
        'Sec-Fetch-Site': 'cross-site',
        'Sec-Fetch-Mode': 'cors',
        'Sec-Fetch-Dest': 'empty',
        'Accept-Encoding': 'gzip, deflate, br',
        'Priority': 'u=1, i',
        'Connection': 'keep-alive'
    }

    response = requests.get(url, headers=headers, params=params, verify=True)
    return response.json()

async def get_dipul_token():
    # curl --path-as-is -i -s -k -X $'GET' \
    # -H $'Host: uas-betrieb.dfs.de' -H $'Sec-Ch-Ua-Platform: \"macOS\"' -H $'Accept-Language: en-US,en;q=0.9' -H $'Accept: application/json, text/plain, */*' -H $'Sec-Ch-Ua: \"Not A(Brand\";v=\"8\", \"Chromium\";v=\"132\"' -H $'User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36' -H $'Sec-Ch-Ua-Mobile: ?0' -H $'Origin: https://maptool-dipul.dfs.de' -H $'Sec-Fetch-Site: same-site' -H $'Sec-Fetch-Mode: cors' -H $'Sec-Fetch-Dest: empty' -H $'Referer: https://maptool-dipul.dfs.de/' -H $'Accept-Encoding: gzip, deflate, br' -H $'Priority: u=1, i' -H $'Connection: keep-alive' \
    # $'https://uas-betrieb.dfs.de/api/token/v1/anonymous/bmdv/token'
    url = "https://uas-betrieb.dfs.de/api/token/v1/anonymous/bmdv/token"
    headers = {"Host": "uas-betrieb.dfs.de", "Sec-Ch-Ua-Platform": "\"macOS\"", "Accept-Language": "en-US,en;q=0.9", "Accept": "application/json, text/plain, */*", "Sec-Ch-Ua": "\"Not A(Brand\";v=\"8\", \"Chromium\";v=\"132\"", "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/"}
    response = requests.get(url, headers=headers, verify=True)
    if response.status_code == 200:
        rjson = response.json()
        return rjson['token']
    else:
        return None

async def post_dipul_affected_areas(lat, lon):
    token = await get_dipul_token()
    if not token:
        return []
    url = "https://dipul-service.dfs.de/api/geoapi/dipul/v2/affectedAreas/typeCode/count"
    headers = {
        'Host': 'dipul-service.dfs.de',
        'Content-Length': '154',
        'Sec-Ch-Ua-Platform': 'macOS',
        'Accept-Language': 'en-US,en;q=0.9',
        'Accept': 'application/json, text/plain, */*',
        'Sec-Ch-Ua': '"Not?A_Brand";v="99", "Chromium";v="130"',
        'Authorization': f'Bearer {token}',
        'Content-Type': 'application/json',
        'Sec-Ch-Ua-Mobile': '?0',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.6723.70 Safari/537.36',
        'Origin': 'https://maptool-dipul.dfs.de',
        'Sec-Fetch-Site': 'same-site',
        'Sec-Fetch-Mode': 'cors',
        'Sec-Fetch-Dest': 'empty',
        'Referer': 'https://maptool-dipul.dfs.de/',
        'Accept-Encoding': 'gzip, deflate, br',
        'Priority': 'u=1, i'
    }
    data = {
        "type": "Feature",
        "properties":{
            "radius":100,
            "subType": "Circle",
            "altitude":{ 
                "value": 0,
                "altitudeReference":"AGL",
                "unit":"m"
                }
        },
        "geometry": {
            "type": "Point",
            "coordinates": [lon, lat]
        }
    }
    logging.debug(f"[/post_dipul_affected_areas] Requesting affected areas for lat: {lat}, lon: {lon}")
    response = requests.post(url, headers=headers, json=data, verify=True)
    logging.debug(f"[/post_dipul_affected_areas] Response: {response.json()}")
    return response.json()

async def get_dipul_detailed_area(typeCode, lat, lon):
    token = await get_dipul_token()
    if not token:
        return False
    url = "https://dipul-service.dfs.de/api/geoapi/dipul/v2/affectedAreas"
    params = {
        'typeCode': typeCode
    }
    headers = {
        'Host': 'dipul-service.dfs.de',
        'Content-Length': '154',
        'Sec-Ch-Ua-Platform': 'macOS',
        'Accept-Language': 'en-US,en;q=0.9',
        'Accept': 'application/json, text/plain, */*',
        'Authorization': f'Bearer {token}',
        'Sec-Ch-Ua': '"Not?A_Brand";v="99", "Chromium";v="130"',
        'Content-Type': 'application/json',
        'Sec-Ch-Ua-Mobile': '?0',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.6723.70 Safari/537.36',
        'Sec-Fetch-Site': 'same-site',
        'Sec-Fetch-Mode': 'cors',
        'Sec-Fetch-Dest': 'empty',
        'Accept-Encoding': 'gzip, deflate, br',
        'Priority': 'u=1, i'
    }
    data = {
        "type": "Feature",
        "properties":{
            "radius":100,
            "subType": "Circle",
            "altitude":{ 
                "value": 0,
                "altitudeReference":"AGL",
                "unit":"m"
                }
        },
        "geometry": {
            "type": "Point",
            "coordinates": [lon, lat]
        }
    }

    response = requests.post(url, headers=headers, params=params, json=data, verify=True)
    return response.json()

async def post_utm_weather_forecast(longitude, latitude):
    url = "https://utm-service.dfs.de/api/weather/v1/weather"
    headers = {
        'Host': 'utm-service.dfs.de',
        'Content-Length': '1948',
        'Sec-Ch-Ua-Platform': 'macOS',
        'Accept-Language': 'en-US,en;q=0.9',
        'Accept': 'application/json, text/plain, */*',
        'Sec-Ch-Ua': '"Not?A_Brand";v="99", "Chromium";v="130"',
        'Content-Type': 'application/json',
        'Sec-Ch-Ua-Mobile': '?0',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.6723.70 Safari/537.36',
        'Sec-Fetch-Site': 'same-site',
        'Sec-Fetch-Mode': 'cors',
        'Sec-Fetch-Dest': 'empty',
        'Accept-Encoding': 'gzip, deflate, br',
        'Priority': 'u=1, i'
    }

    # Generate forecast times starting from the current time
    berlin_tz = pytz.timezone('Europe/Berlin')

    # Get the current date and time in Berlin
    current_time = datetime.now(berlin_tz)
    forecasts = [(current_time + timedelta(hours=i)).replace(tzinfo=None).isoformat() + 'Z' for i in range(3)]

    data = {
        "positions": [
            {
                "longitude": longitude,
                "latitude": latitude,
                "forecasts": forecasts,
                "temperature": {
                    "unit": "C",
                    "heights": [
                        {"reference": "AGL", "unit": "m", "value": 2},
                        {"reference": "AGL", "unit": "m", "value": 50},
                        {"reference": "AGL", "unit": "m", "value": 100},
                        {"reference": "AGL", "unit": "m", "value": 150}
                    ]
                },
                "wind": {
                    "unit": "m/s",
                    "heights": [
                        {"reference": "AGL", "unit": "m", "value": 10},
                        {"reference": "AGL", "unit": "m", "value": 50},
                        {"reference": "AGL", "unit": "m", "value": 100},
                        {"reference": "AGL", "unit": "m", "value": 150}
                    ]
                },
                "rainPrecipitation": {"unit": "mm"},
                "snowPrecipitation": {"unit": "cm"},
                "totalCloudCover": {"unit": "%"},
                "cloudCover": {
                    "unit": "%",
                    "heights": [
                        {"reference": "AGL", "unit": "m", "value": 50},
                        {"reference": "AGL", "unit": "m", "value": 100},
                        {"reference": "AGL", "unit": "m", "value": 150}
                    ]
                },
                "airHumidity": {
                    "unit": "%",
                    "heights": [
                        {"reference": "AGL", "unit": "m", "value": 50},
                        {"reference": "AGL", "unit": "m", "value": 100},
                        {"reference": "AGL", "unit": "m", "value": 150}
                    ]
                }
            }
        ]
    }

    response = requests.post(url, headers=headers, json=data, verify=True)
    return response.json()

async def get_openweather_weather(lat, lon):

    url = "https://api.openweathermap.org/data/3.0/onecall"
    params = {
        'lat': lat,
        'lon': lon,
        'exclude': 'minutely,hourly,daily',
        'appid': openweather_token,
        'units': 'metric'
    }
    response = requests.get(url, params=params)
    return response.json()

def extract_location_info(geocode_data: Dict[str, Any]) -> Dict[str, Any]:
    item = geocode_data['items'][0]
    return {
        'city': item['address']['city'],
        'street': item['address']['street'],
        'postalCode': item['address']['postalCode'],
        'houseNumber': item['address'].get('houseNumber', ''),
        'lat': item['position']['lat'],
        'lon': item['position']['lng']
    }

def format_location_info(location_info: Dict[str, Any]) -> str:
    return f"Flug Details für:\n<b>{location_info['street']} {location_info['houseNumber']}</b>\n<b>{location_info['postalCode']} {location_info['city']}</b>\n\n"

async def format_affected_areas(affected_areas: Dict[str, Any], lat: float, lon: float) -> str:
    txt = ""
    categories = set()
    ret_json = {}
    for typeCode in affected_areas:
        if typeCode not in categories:
            categories.add(typeCode)
        else:
            continue
        detailed_area = await get_dipul_detailed_area(typeCode, lat, lon)
        total_records = detailed_area['totalRecords']
        affected_areas = detailed_area['affectedAreas']
        if affected_areas:
            ret_json[affected_areas[0]['typeCode']] = {"totalRecords": total_records, "affectedAreas": affected_areas}
    return ret_json


async def format_weather_json(weather_forecast: Dict[str, Any], openweather: Dict[str, Any], kp_index) -> Dict[str, Any]:
    # return weather_forecast as json
    ret_json = {}
    ret_json['temperature'] = {}
    ret_json['temperature_warning'] = []
    ret_json['windSpeedGust'] = {}
    ret_json['windSpeed'] = {}
    ret_json['wind_warning'] = []
    ret_json['rainPrecipitation'] = {}
    ret_json['snowPrecipitation'] = {}
    ret_json['totalCloudCover'] = {}
    ret_json['kp_index'] = {'value' : kp_index,  'flyable': True}
    ret_json['kp_warning'] = []
    ret_json['flyable'] = True

    flyable = True
    temperature = weather_forecast['positions'][0]['forecasts'][0]['temperature']
    wind = weather_forecast['positions'][0]['forecasts'][0]['wind']
    rainPrecipitation = weather_forecast['positions'][0]['forecasts'][0]['rainPrecipitation']
    snowPrecipitation = weather_forecast['positions'][0]['forecasts'][0]['snowPrecipitation']
    totalCloudCover = weather_forecast['positions'][0]['forecasts'][0]['totalCloudCover']

    if 'current' not in openweather:
        print("No current weather data")
        dew_point = "N/A"
        ret_json['dew_point'] = {"value": dew_point, "flyable": True}
    else:
        dew_point = openweather['current']['dew_point']
        ret_json['dew_point'] = {"value": dew_point, "flyable": True}

    temp_ok = True
    for pos in temperature:
        height = pos['height']['value']
        unit = pos['height']['unit']
        
        if pos['value'] > 50:
            ret_json['temperature_warning'].append(f"{round(pos['value'], 2)} °C [{height}{unit}]")
            flyable = False
            temp_ok = False
            ret_json['temperature'][f"{height}{unit}"] = {"value": round(pos['value'],2), "flyable": False}
            
        if pos['value'] < -20:
            flyable = False
            temp_ok = False
            ret_json['temperature'][f"{height}{unit}"] = {"value": round(pos['value'],2), "flyable": False}
            ret_json['temperature_warning'].append(f"{round(pos['value'], 2)} [{height}{unit}] °C")
        
        if temp_ok:
            ret_json['temperature'][f"{height}{unit}"] = {"value": round(pos['value'],2), "flyable": True}
        
        if round(dew_point, 2) is not None and round(dew_point, 2) != "N/A" and abs(round(pos['value'],2) - round(dew_point, 2)) < 2:
            if round(pos['value'],2) < 7:
                ret_json['temperature_warning'].append(f"{round(pos['value'], 2)} °C (TP: {round(dew_point, 2)} °C) [{height}{unit}]")
                flyable = False
                ret_json['dew_point'] = {"value": round(dew_point,2), "flyable": False}
    
    wind_ok = True
    for pos in wind:
        height = pos['height']['value']
        unit = pos['height']['unit']
        if pos.get('vComponent') is not None and abs(pos['vComponent']) > 12:
            ret_json['wind_warning'].append(f"{round(pos['vComponent'], 2)} m/s [{height}{unit}]")
            ret_json['windSpeed'][f"{height}{unit}"] = {"value": abs(round(pos['vComponent'],2)), "flyable": False}
            flyable = False
            wind_ok = False
        if wind_ok:
            ret_json['windSpeed'][f"{height}{unit}"] = {"value": abs(round(pos['vComponent'],2)), "flyable": True}
    
    if abs(wind[0]['windSpeedGust']) > 12:
        ret_json['wind_warning'].append(f"[{abs(round(wind[0]['windSpeedGust'], 2))} m/s] Böen")
        flyable = False
        ret_json["windSpeedGust"] = {"value": abs(round(wind[0]['windSpeedGust'],2)), "flyable": False}
    else:
        ret_json["windSpeedGust"] = {"value": round(wind[0]['windSpeedGust'],2), "flyable": True}
    try:
        if ret_json['kp_index']['value'] != 'N/A' and ret_json['kp_index']['value'] > 4:
            ret_json['kp_warning'].append(f"Kp-Index: {ret_json['kp_index']['value']}")
            flyable = False
            ret_json['kp_index']['flyable'] = False
    except Exception as e:
        logging.error(f"Error KP Index Parsing: {e}")

        
    ret_json['rainPrecipitation'] = round(rainPrecipitation['value'],2)
    ret_json['snowPrecipitation'] = round(snowPrecipitation['value'],2)
    ret_json['totalCloudCover'] = round(totalCloudCover['value'],2)
    ret_json['flyable'] = flyable
    print(ret_json)
    return ret_json


# # Route for favicon
@app.get("/favicon.ico", include_in_schema=False)
async def favicon():
    return FileResponse("static/favicon.ico")

@app.head("/", include_in_schema=False)
async def read_root_head():
    # Custom response for HEAD (only headers)
    return {"status": "alive"}

@app.get("/health", include_in_schema=False)
async def health():
    return {"status": "alive"}

@limiter.limit("5/minute")
@app.get("/", response_class=HTMLResponse)
async def home(request: Request):
    ip_address = request.client.host
    user_agent = request.headers['user-agent']
    # cloudfare_ip 
    cf = request.headers.get('cf-connecting-ip')
    if cf:
        ip_address = cf
    logging.info(f"[/] Request from {ip_address} with user agent {user_agent}")
    return templates.TemplateResponse("index.html", {"request": request})

@limiter.limit("5/minute")
@app.post("/results", response_class=HTMLResponse)
#user: dict = Depends(basic_auth)
async def results(request: Request, address: str = Form(...)):
    ip_address = request.client.host
    user_agent = request.headers['user-agent']
    # cloudfare_ip 
    cf = request.headers.get('cf-connecting-ip')
    if cf:
        ip_address = cf
    logging.info(f"[/results] Request from {ip_address} with user agent {user_agent}")
    if len(address) == 0:
        return templates.TemplateResponse("results.html", {"request": request, "error": "Bitte geben Sie eine Adresse ein."})
    if len(address) > 100:
        address = address[:100]
    logging.info(f"[/results] Received address: {address}")
    try:
        geocode_data = await get_hereapi_geocode(address)
        if not geocode_data['items']:
            return templates.TemplateResponse("results.html", {"request": request, "error": "Adresse nicht gefunden."})
        
        logging.debug(f"[/results] Geocode data: {geocode_data}")
        try:
            location = geocode_data['items'][0]
            lat, lon = location['position']['lat'], location['position']['lng']
        except Exception as e:
            logging.error(f"[/results] Error parsing geocode data: {e}")
            return templates.TemplateResponse("results.html", {"request": request, "error": "Ein Fehler ist aufgetreten."})
        try:
            weather_utm_forecast = await post_utm_weather_forecast(lon, lat)
        except Exception as e:
            logging.error(f"[/results] Error fetching weather utm forecast: {e}")
            return templates.TemplateResponse("results.html", {"request": request, "error": "Ein Fehler ist aufgetreten."})
        try:
            weather_openweather = await get_openweather_weather(lat, lon)
        except Exception as e:
            logging.error(f"[/results] Error fetching openweather forecast: {e}")
            return templates.TemplateResponse("results.html", {"request": request, "error": "Ein Fehler ist aufgetreten."})
        currentTime = datetime.now()
        to_date = currentTime.strftime('%Y-%m-%dT%H:%M:%SZ')
        from_date = (currentTime - timedelta(days=1)).strftime('%Y-%m-%dT%H:%M:%SZ')
        try:
            kp_index = await get_getKpindex(from_date, to_date, 'Kp')
        except Exception as e:
            logging.error(f"[/results] Error fetching KP index: {e}")
            return templates.TemplateResponse("results.html", {"request": request, "error": "Ein Fehler ist aufgetreten."})
        try:
            weather_formatted = await format_weather_json(weather_utm_forecast, weather_openweather, kp_index)
        except Exception as e:
            logging.error(f"[/results] Error formatting weather data: {e}")
            return templates.TemplateResponse("results.html", {"request": request, "error": "Ein Fehler ist aufgetreten."})
        try:
            affected_areas = await post_dipul_affected_areas(lat, lon)
        except Exception as e:
            logging.error(f"[/results] Error fetching affected areas: {e}")
            return templates.TemplateResponse("results.html", {"request": request, "error": "Ein Fehler ist aufgetreten."})
        try:
            affected_areas_formatted = await format_affected_areas(affected_areas, lat, lon)
        except Exception as e:
            logging.error(f"[/results] Error formatting affected areas: {e}")
            return templates.TemplateResponse("results.html", {"request": request, "error": "Ein Fehler ist aufgetreten."})
        logging.info(f"[/results] Resolved Address: {location.get('title', address)}")
        return templates.TemplateResponse("results.html", {
            "request": request,
            "address": location.get('title', address),
            "lat": lat,
            "lon": lon,
            "affected_areas": affected_areas_formatted,
            "forecast": weather_formatted
        })
    except Exception as e:
        logging.error(f"[/results] Error: {e}")
        return templates.TemplateResponse("results.html", {"request": request, "error": "Ein Fehler ist aufgetreten."})
    

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="localhost", port=8000)
