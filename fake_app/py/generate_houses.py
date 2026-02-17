# -*- coding: utf-8 -*-
"""
基于 data 目录下地标文件（subway_stations, landmarks, fortune500_companies）生成租房数据，
房源坐标与地铁/地标真实对应。本次生成数量由参数传入，追加写入；按 2000 条分文件：
database_2000.json(1~2000) -> database_4000.json(2001~4000) -> database_6000.json -> database_8000.json -> database_10000.json。
"""
import json
import random
import math
import os
import argparse

# 脚本在 fake_app/py/，数据在 fake_app/data/
_SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DATA_DIR = os.path.normpath(os.path.join(_SCRIPT_DIR, "..", "data"))
# 每文件最多 2000 条；超过后新建 database_4000 / 6000 / 8000 / 10000
FILE_CAP = 2000
OUTPUT_FILES = [
    os.path.join(DATA_DIR, "database_2000.json"),
    os.path.join(DATA_DIR, "database_4000.json"),
    os.path.join(DATA_DIR, "database_6000.json"),
    os.path.join(DATA_DIR, "database_8000.json"),
    os.path.join(DATA_DIR, "database_10000.json"),
]
SUBWAY_PATH = os.path.join(DATA_DIR, "subway_stations.json")
LANDMARKS_PATH = os.path.join(DATA_DIR, "landmarks.json")
F500_PATH = os.path.join(DATA_DIR, "fortune500_companies.json")

# 西二旗站坐标，用于计算 commute_to_xierqi
XIERQI_LON, XIERQI_LAT = 116.3289, 40.0567

# 行政区基准单价（元/平米/月）
# 参考：中国房价行情网、北京住房租赁市场监测月报等，2024-2025 年水平：
# 西城约 146、东城/朝阳/海淀 120+、全市住宅均价约 83；远郊约为核心区 1/3～1/2。
# 以下为按该口径设定的基准，便于生成与区位温和的月租金。
DISTRICT_BASE_PRICE = {
    "东城": 128, "西城": 142, "朝阳": 118, "海淀": 122,
    "丰台": 72, "石景山": 65, "通州": 52, "大兴": 48,
    "昌平": 46, "顺义": 44, "房山": 40, "门头沟": 38,
}
DEFAULT_BASE_PRICE = 45

def haversine_km(lon1, lat1, lon2, lat2):
    R = 6371
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlam = math.radians(lon2 - lon1)
    a = math.sin(dphi/2)**2 + math.cos(phi1)*math.cos(phi2)*math.sin(dlam/2)**2
    c = 2 * math.atan2(math.sqrt(a), math.sqrt(1-a))
    return R * c

def commute_minutes(km):
    return max(8, min(95, int(round(km * 2.2))))

# 小区名成分（与行政区搭配生成不重复名称）
COMMUNITY_SUFFIXES = ["家园", "里", "小区", "苑", "园", "嘉园", "花园", "庭", "府", "居", "舍", "轩", "阁", "湾", "悦", "锦园", "华府", "国际", "公馆"]
COMMUNITY_PREFIXES = ["阳光", "锦绣", "万科", "保利", "金地", "龙湖", "华润", "融创", "绿城", "中海", "远洋", "首开", "城建", "天鸿", "世纪", "恒通", "博雅", "雅居", "幸福", "和平", "康乐", "怡景", "翠湖", "紫金", "银座", "万达", "恒基", "润泽", "观澜", "御景"]

# 朝向、整租合租 平均分布（各选项等概率）
ORIENTATIONS = ["朝南", "朝北", "朝东", "朝西", "南北", "东西"]  # 6 种均分
# 装修 含无装修(毛坯)、空房；轮换 简装32% 精装45% 豪华12% 毛坯8% 空房5%
DECORATION_ROTATE_25 = (
    ["简装"] * 8 + ["精装"] * 11 + ["豪华"] * 3 + ["毛坯"] * 2 + ["空房"] * 1
)  # 25 个，index % 25
FLOOR_OPTIONS = ["低层", "中层", "高层"]
# 规范：floor 可为「低层/中层/高层」或「共N层」；价格系数见 FLOOR_BONUS，共N层按中层处理
NOISE_OPTIONS = ["安静", "中等", "吵闹", "临街"]

# 整租：全部「几室几厅几卫」场景 + 对应合理面积区间（参考市场：一居室为一室一厅一卫，无两卫）
# 每项 (bedrooms, livingrooms, bathrooms, area_min, area_max)
LAYOUT_SCENARIOS = [
    (1, 1, 1, 22, 52),   # 一居室标准：1室1厅1卫，面积一般不超过 60 平
    (2, 0, 1, 48, 58),
    (2, 1, 1, 55, 75),
    (2, 1, 2, 70, 85),
    (2, 2, 1, 75, 88),
    (2, 2, 2, 82, 92),
    (3, 0, 1, 78, 90),
    (3, 0, 2, 88, 98),
    (3, 1, 1, 85, 105),
    (3, 1, 2, 95, 115),
    (3, 2, 1, 100, 118),
    (3, 2, 2, 108, 125),
    (4, 0, 1, 105, 118),
    (4, 0, 2, 112, 122),
    (4, 1, 1, 115, 128),
    (4, 1, 2, 120, 135),
    (4, 2, 1, 125, 138),
    (4, 2, 2, 130, 145),
]

# 合租：仅「单间」场景，bedrooms=整套室数，area_sqm=单间面积
HEZHU_LAYOUTS = [
    (2, 1, 1), (2, 1, 2), (3, 1, 1), (3, 1, 2), (4, 1, 1), (4, 1, 2),
]

# 整租/合租 平均分布（50% : 50%）
PROPERTY_TYPES = ["住宅", "住宅", "住宅", "公寓"]  # 住宅为主
# 住宅一律民水民电；公寓可为商水商电（见 gen_house 内按 property_type 分配）
UTILITIES_民用 = "民水民电"
UTILITIES_商用电 = "商水商电"
PLATFORMS = ["安居客", "链家", "58同城"]
LISTING_URLS = ["https://bj.zu.anjuke.com/", "https://bj.lianjia.com/", "https://bj.58.com/"]

# 地铁距离分段，用于价格系数与覆盖：近/中近/中/远/很远
SUBWAY_DISTANCE_BANDS = [
    (200, 500, 1.08),   # 近地铁 溢价
    (500, 1000, 1.0),
    (1000, 2000, 0.92),
    (2000, 3500, 0.85),
    (3500, 5500, 0.78),
]
DECORATION_MULTIPLIER = {"简装": 1.0, "精装": 1.28, "豪华": 1.55, "毛坯": 0.78, "空房": 0.88}  # 毛坯/空房价格明显更低
ELEVATOR_BONUS = 1.03  # 有电梯略贵
FLOOR_BONUS = {"高层": 1.02, "中层": 1.0, "低层": 0.98}  # 高层景观略贵；「共N层」get 不到则 1.0
RANDOM_PRICE_VARIANCE = 0.06  # 价格 ±6% 随机

def load_json(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)

def save_json(path, data):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

def get_current_output():
    """返回 (当前写入文件路径, 当前文件中的 houses, 下一个 house_id, 全部已有 houses)。
    会按顺序读取 database_2000/4000/6000/8000/10000，把所有已存在房源的 houses 合并到 all_houses，
    保证 n_existing、index_for_coverage、used_communities 都基于「全量已有数据」，从而任意次数、任意数量追加后整体分布仍按规则。"""
    total = 0
    all_houses = []
    for path in OUTPUT_FILES:
        if not os.path.exists(path):
            return path, [], total + 1, all_houses
        data = load_json(path)
        houses = data.get("houses", [])
        all_houses.extend(houses)  # 跨文件累积，保证 all_houses = 所有已存在房源
        total += len(houses)
        if len(houses) < FILE_CAP:
            return path, houses, total + 1, all_houses
    return None, None, None, all_houses  # 已达 10000 条

def get_next_file_path(current_path):
    """当前文件写满后的下一个文件路径；若无则返回 None。"""
    try:
        i = OUTPUT_FILES.index(current_path)
        if i + 1 < len(OUTPUT_FILES):
            return OUTPUT_FILES[i + 1]
    except ValueError:
        pass
    return None

def build_station_map(subway):
    by_name = {}
    for s in subway["stations"]:
        name = s["name"]
        lines = s.get("lines", [])
        by_name[name] = {
            "district": s["district"],
            "name": name,
            "line": lines[0] if lines else "地铁",
            "lines": lines,
            "longitude": s["longitude"],
            "latitude": s["latitude"],
        }
    return by_name

def build_seeds(subway, landmarks, companies, station_by_name):
    seeds = []
    for s in subway["stations"]:
        seeds.append({
            "district": s["district"],
            "area": s["name"].replace("站", ""),
            "subway_station": s["name"],
            "subway": s["lines"][0] if s.get("lines") else "地铁",
            "longitude": s["longitude"],
            "latitude": s["latitude"],
        })
    for lm in landmarks.get("landmarks", []):
        station_name = lm.get("nearby_subway", "")
        if station_name and station_name in station_by_name:
            st = station_by_name[station_name]
            seeds.append({
                "district": lm["district"],
                "area": lm["name"][:4] if len(lm["name"]) >= 4 else lm["district"],
                "subway_station": station_name,
                "subway": st["line"],
                "longitude": lm["longitude"],
                "latitude": lm["latitude"],
            })
    for co in companies.get("companies", []):
        station_name = co.get("nearby_subway", "")
        if station_name and station_name in station_by_name:
            st = station_by_name[station_name]
            seeds.append({
                "district": co["district"],
                "area": co.get("short_name", co["district"]),
                "subway_station": station_name,
                "subway": st["line"],
                "longitude": co["longitude"],
                "latitude": co["latitude"],
            })
    return seeds

def random_offset(center_lon, center_lat, radius_km=0.8):
    # 在中心点附近随机偏移，约 radius_km 公里内
    angle = random.uniform(0, 2 * math.pi)
    r = random.uniform(0.1, radius_km)
    # 1度纬度约 111km, 1度经度约 85km @ 40N
    dlat = (r / 111) * math.cos(angle) * random.uniform(0.5, 1.5)
    dlon = (r / 85) * math.sin(angle) * random.uniform(0.5, 1.5)
    return center_lon + dlon, center_lat + dlat

def gen_community(used_names, district):
    for _ in range(100):
        pre = random.choice(COMMUNITY_PREFIXES)
        suf = random.choice(COMMUNITY_SUFFIXES)
        name = pre + suf
        if random.random() < 0.3:
            name += str(random.randint(1, 9)) + "区"
        if name not in used_names:
            used_names.add(name)
            return name
    return district + "小区" + str(random.randint(100, 999))

def pick_subway_distance_band(index_for_coverage):
    """按索引轮换距离分段，保证近/中/远全面覆盖。"""
    band_idx = index_for_coverage % len(SUBWAY_DISTANCE_BANDS)
    low, high, _ = SUBWAY_DISTANCE_BANDS[band_idx]
    return random.randint(int(low), int(high)), band_idx

def build_tags_from_data(
    decoration,
    subway_dist,
    orientation,
    has_elevator,
    area_sqm,
    district,
    rental_type=None,
    bedrooms=None,
    bathrooms=None,
    floor_choice=None,
    property_type=None,
    price=None,
    subway_str=None,
    area_name=None,
):
    """仅根据房源实际字段推导 tags（与 0216bak 全量 tag 对齐），不随机。"""
    tags = []
    # 租型
    if rental_type == "合租":
        tags.append("合租")
        if area_sqm is not None and area_sqm < 25:
            tags.append("小单间")
    # 装修
    if decoration in ("精装", "豪华"):
        tags.append("精装修")
    if decoration == "豪华":
        tags.append("豪华装修")
    elif decoration == "毛坯":
        tags.append("毛坯")
    elif decoration == "空房":
        tags.append("空房")
    # 地铁
    if subway_dist is not None and subway_dist <= 800:
        tags.append("近地铁")
    if subway_str and "/" in str(subway_str):
        n = len(str(subway_str).split("/"))
        if n >= 2:
            tags.append("双地铁" if n == 2 else "多地铁")
    # 朝向 / 采光
    if orientation == "朝南":
        tags.append("朝南")
    if orientation == "南北":
        tags.append("南北通透")
    if orientation in ("朝南", "南北"):
        tags.append("采光好")
    # 电梯 / 楼层
    if has_elevator:
        tags.append("有电梯")
    if floor_choice == "高层":
        tags.append("高楼层")
        tags.append("高层")
    # 面积 -> 户型（整租看整套面积，合租 area_sqm 已是单间）
    if area_sqm is not None:
        if area_sqm >= 100:
            tags.append("大户型")
        elif area_sqm < 60:
            tags.append("小户型")
        if rental_type != "合租":
            if bedrooms is not None and bedrooms == 2 and area_sqm >= 70:
                tags.append("大两居")
            if bedrooms is not None and bedrooms == 3 and area_sqm >= 90:
                tags.append("大三居")
    # 卫
    if bathrooms is not None and bathrooms >= 2:
        tags.append("双卫")
    # 区位
    if district in ("东城", "西城", "朝阳", "海淀"):
        tags.append("核心区")
    if district in ("海淀", "西城"):
        tags.append("学区房")
    if district == "海淀":
        tags.append("近高校")
    if area_name in ("西二旗", "上地"):
        tags.append(area_name)
    if area_name and "朝阳路" in str(area_name):
        tags.append("朝阳路")
    # 物业类型
    if property_type == "公寓":
        tags.append("商住")
    # 价格：相对参考价 + 绝对月租上限（保留 低价 + 高性价比，已移除“价格实惠”）
    # 低价：相对参考价 0.85 倍以下 且 绝对<4500（之前原则）；高性价比<6500
    if price is not None and rental_type == "整租" and area_sqm and area_sqm > 0:
        base = DISTRICT_BASE_PRICE.get(district, DEFAULT_BASE_PRICE)
        ref = base * area_sqm * 0.92
        if price < ref * 0.85 and price < 4500:
            tags.append("低价")
        elif price < ref * 1.05 and price < 6500:
            tags.append("高性价比")
    # 远郊低价可打农村房（房山/门头沟等 + 低价）
    if district in ("房山", "门头沟") and price is not None and area_sqm and area_sqm < 50:
        if price < 2500:
            tags.append("农村房")
        if price < 2000:
            tags.append("农村自建房")
    return tags

def calc_price(district, area_sqm, decoration, subway_dist, has_elevator, floor_choice):
    """根据区位、面积、装修、地铁距离、电梯、楼层计算整租月租金。"""
    base = DISTRICT_BASE_PRICE.get(district, DEFAULT_BASE_PRICE)
    dec_mult = DECORATION_MULTIPLIER.get(decoration, 1.2)
    sub_mult = 1.0
    for low, high, mult in SUBWAY_DISTANCE_BANDS:
        if low <= subway_dist < high:
            sub_mult = mult
            break
    if subway_dist >= 5500:
        sub_mult = 0.78
    elev_mult = ELEVATOR_BONUS if has_elevator else 1.0
    floor_mult = FLOOR_BONUS.get(floor_choice, 1.0)
    variance = 1.0 + random.uniform(-RANDOM_PRICE_VARIANCE, RANDOM_PRICE_VARIANCE)
    price = base * area_sqm * dec_mult * sub_mult * elev_mult * floor_mult * variance
    price = max(800, min(28000, int(price)))
    return (price // 50) * 50

# 合租单间面积 12～30 平米（与 0216bak 真实数据 13～25 一致）
HEZHU_AREA_RANGE = (12, 30)
# 合租单间月租 1200～3500 元（与 0216bak 1600～3050 对齐）
HEZHU_PRICE_RANGE = (1200, 3500)

def calc_price_hezhu(district, area_sqm, decoration, subway_dist, has_elevator):
    """合租单间月租：按区位/单间面积/装修/地铁/电梯计算，落在 1200～3500。"""
    base = DISTRICT_BASE_PRICE.get(district, DEFAULT_BASE_PRICE) * 1.35  # 单间单价略高于整租均摊
    dec_mult = DECORATION_MULTIPLIER.get(decoration, 1.2)
    sub_mult = 1.0
    for low, high, mult in SUBWAY_DISTANCE_BANDS:
        if low <= subway_dist < high:
            sub_mult = mult
            break
    if subway_dist >= 5500:
        sub_mult = 0.78
    elev_mult = ELEVATOR_BONUS if has_elevator else 1.0
    variance = 1.0 + random.uniform(-RANDOM_PRICE_VARIANCE, RANDOM_PRICE_VARIANCE)
    price = base * area_sqm * dec_mult * sub_mult * elev_mult * variance
    price = max(HEZHU_PRICE_RANGE[0], min(HEZHU_PRICE_RANGE[1], int(price)))
    return (price // 50) * 50

def gen_house(house_id, seed, used_communities, index_for_coverage):
    lon, lat = random_offset(seed["longitude"], seed["latitude"])
    # 先定租型，再定户型与面积（合租=单间面积 12～30，整租=整套面积）
    orientation = ORIENTATIONS[index_for_coverage % len(ORIENTATIONS)]
    rental_type = "合租" if (index_for_coverage % 2 == 0) else "整租"  # 50% : 50%
    hidden_noise = NOISE_OPTIONS[index_for_coverage % len(NOISE_OPTIONS)]

    if rental_type == "合租":
        # 合租：轮换所有合租场景（整套几室几厅几卫），area_sqm 为单间面积 12～30
        layout = HEZHU_LAYOUTS[index_for_coverage % len(HEZHU_LAYOUTS)]
        bedrooms, livingrooms, bathrooms = layout[0], layout[1], layout[2]
        area_sqm = int(round(random.uniform(HEZHU_AREA_RANGE[0], HEZHU_AREA_RANGE[1]), 0))
    else:
        # 整租：轮换全部 LAYOUT_SCENARIOS，保证不缺少任一种几室几厅几卫，面积在对应区间内
        scenario = LAYOUT_SCENARIOS[index_for_coverage % len(LAYOUT_SCENARIOS)]
        bedrooms, livingrooms, bathrooms = scenario[0], scenario[1], scenario[2]
        area_min, area_max = scenario[3], scenario[4]
        area_sqm = int(round(random.uniform(area_min, area_max), 0))

    # 地铁距离：轮换分段以保证近/中/远全面覆盖
    subway_dist, _ = pick_subway_distance_band(index_for_coverage)
    total_floors = random.choice([4, 5, 6, 11, 18, 22, 28, 32])
    floor_choice = FLOOR_OPTIONS[index_for_coverage % len(FLOOR_OPTIONS)]
    if total_floors <= 6 and (index_for_coverage // 3) % 4 == 0:
        floor_choice = "共%d层" % total_floors
    has_elevator = total_floors >= 6 and random.random() < 0.82
    decoration = DECORATION_ROTATE_25[index_for_coverage % 25]

    if rental_type == "合租":
        price = calc_price_hezhu(seed["district"], area_sqm, decoration, subway_dist, has_elevator)
    else:
        price = calc_price(seed["district"], area_sqm, decoration, subway_dist, has_elevator, floor_choice)

    plat_idx = random.randint(0, 2)
    # 可入住日期统一为 2 月 14 号以后（2026-02-15 起 0～45 天）
    from datetime import datetime, timedelta
    available_days = random.randint(0, 45)
    d = datetime(2026, 2, 15) + timedelta(days=available_days)
    available_from = d.strftime("%Y-%m-%d")

    km = haversine_km(lon, lat, XIERQI_LON, XIERQI_LAT)
    commute = commute_minutes(km)

    community = gen_community(used_communities, seed["district"])
    street_num = random.randint(1, 99)
    area_name = seed.get("area") or seed.get("district") or ""
    address = (area_name + "路" + str(street_num) + "号") if area_name else (seed.get("district", "") + "路" + str(street_num) + "号")

    property_type = random.choice(PROPERTY_TYPES)
    # tags 仅由实际字段推导（与 0216bak 全量 tag 对齐）
    tags = build_tags_from_data(
        decoration,
        subway_dist,
        orientation,
        has_elevator,
        area_sqm,
        seed["district"],
        rental_type=rental_type,
        bedrooms=bedrooms,
        bathrooms=bathrooms,
        floor_choice=floor_choice,
        property_type=property_type,
        price=price,
        subway_str=seed.get("subway"),
        area_name=seed.get("area"),
    )

    house = {
        "house_id": "HF_%d" % house_id,
        "community": community,
        "district": seed.get("district", ""),
        "area": seed.get("area", seed.get("district", "")),
        "address": address,
        "bedrooms": bedrooms,
        "livingrooms": livingrooms,
        "bathrooms": bathrooms,
        "area_sqm": area_sqm,
        "floor": floor_choice,
        "total_floors": total_floors,
        "orientation": orientation,
        "decoration": decoration,
        "price": price,
        "price_unit": "元/月",
        "rental_type": rental_type,
        "property_type": property_type,
        "utilities_type": UTILITIES_民用 if property_type == "住宅" else (UTILITIES_商用电 if random.random() < 0.6 else UTILITIES_民用),  # 住宅必民用，公寓约 60% 商水商电
        "elevator": has_elevator,
        "subway": seed.get("subway", ""),
        "subway_distance": subway_dist,
        "subway_station": seed.get("subway_station", ""),
        "commute_to_xierqi": commute,
        "available_from": available_from,
        "listing_platform": PLATFORMS[plat_idx],
        "listing_url": LISTING_URLS[plat_idx],
        "tags": tags,
        "hidden_noise_level": hidden_noise,
        # status：available（可租）/ rented（已租出）/ offline（下架）；按 index 轮换约 10% 已出租、约 5% 下架，其余可租
        "status": (
            "rented" if (index_for_coverage % 20 == 0) else
            "offline" if (index_for_coverage % 20 == 1) else
            "available"
        ),
        "longitude": round(lon, 4),
        "latitude": round(lat, 4),
        "coordinate_system": "WGS84",
    }
    return house

def main():
    parser = argparse.ArgumentParser(description="按参数数量生成租房数据并追加；满 2000 条则新建 database_4000/6000/8000/10000.json")
    parser.add_argument("count", type=int, help="本次生成条数")
    args = parser.parse_args()
    need = args.count
    if need <= 0:
        raise SystemExit("count 须为正整数")

    random.seed(42)
    print("加载 data 目录下地标与现有 database...")
    subway = load_json(SUBWAY_PATH)
    landmarks = load_json(LANDMARKS_PATH)
    companies = load_json(F500_PATH)

    current_path, current_houses, next_id, all_existing = get_current_output()
    if current_path is None:
        raise SystemExit("已达最大容量 10000 条，无法再追加。")

    station_by_name = build_station_map(subway)
    seeds = build_seeds(subway, landmarks, companies, station_by_name)
    if not seeds:
        raise SystemExit("未找到地标种子，请检查 subway_stations/landmarks/companies 文件。")
    print("地标种子数:", len(seeds))
    n_existing = len(all_existing)
    print("当前总房源数=%d，本次追加 %d 条，写入文件: %s" % (n_existing, need, os.path.basename(current_path)))

    used_communities = {h["community"] for h in all_existing}

    # 轮换使用各地标种子（按“已有条数+本次序号”轮换，追加时延续覆盖，不重复前段）
    new_houses = []
    for i in range(need):
        seed = seeds[(n_existing + i) % len(seeds)]
        house = gen_house(next_id + i, seed, used_communities, index_for_coverage=n_existing + i)
        new_houses.append(house)
        if (i + 1) % 200 == 0:
            print("已生成", i + 1, "条")

    # 按 2000 条分文件写入：先写满当前文件，再写下一个
    to_add = new_houses
    path = current_path
    houses = current_houses
    data = {"houses": houses}

    while to_add:
        space = FILE_CAP - len(houses)
        chunk = to_add[:space]
        to_add = to_add[space:]
        houses.extend(chunk)
        data["houses"] = houses
        save_json(path, data)
        print("已写入 %s，当前 %d 条" % (os.path.basename(path), len(houses)))
        if to_add:
            path = get_next_file_path(path)
            if path is None:
                raise SystemExit("已达 10000 条上限，剩余 %d 条未写入。" % len(to_add))
            houses = load_json(path).get("houses", []) if os.path.exists(path) else []
            data = {"houses": houses}

    print("完成。本次新增 %d 条，当前文件 %s 共 %d 条。" % (need, os.path.basename(path), len(houses)))

    # 简要统计（仅当前文件）
    if houses:
        prices = [h["price"] for h in houses]
        dists = [h["subway_distance"] for h in houses]
        print("当前文件 价格: %d ~ %d 元/月，地铁距离: %d ~ %d 米" % (min(prices), max(prices), min(dists), max(dists)))

if __name__ == "__main__":
    main()
