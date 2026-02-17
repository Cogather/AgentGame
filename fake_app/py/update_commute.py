# -*- coding: utf-8 -*-
"""根据房源坐标与西二旗站坐标，计算 commute_to_xierqi（分钟）并写回 database.json"""
import json
import math

XIERQI_LON = 116.3289
XIERQI_LAT = 40.0567

def haversine_km(lon1, lat1, lon2, lat2):
    R = 6371  # Earth radius km
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlam = math.radians(lon2 - lon1)
    a = math.sin(dphi/2)**2 + math.cos(phi1)*math.cos(phi2)*math.sin(dlam/2)**2
    c = 2 * math.atan2(math.sqrt(a), math.sqrt(1-a))
    return R * c

def km_to_commute_min(km):
    # 北京地铁+步行约 2.2 分钟/公里，最少 8 分钟，最多 95 分钟
    min_val = max(8, min(95, int(round(km * 2.2))))
    return min_val

def main():
    path = r"d:\赵星code\0215-agent\AgentGame\fake_app\data\database.json"
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)

    updated = 0
    for h in data["houses"]:
        if h.get("commute_to_xierqi") != 0:
            continue
        lon = h["longitude"]
        lat = h["latitude"]
        km = haversine_km(lon, lat, XIERQI_LON, XIERQI_LAT)
        mins = km_to_commute_min(km)
        h["commute_to_xierqi"] = mins
        updated += 1
        print(f"{h['house_id']} {h['community']}: {km:.1f}km -> {mins}min")

    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

    print(f"\nUpdated {updated} houses.")

if __name__ == "__main__":
    main()
