"""
地标数据管理模块
支持加载、查询地铁站、世界500强企业、商圈地标数据
"""

import json
import os
from typing import List, Dict, Optional, Union
from dataclasses import dataclass
from enum import Enum


class LandmarkCategory(Enum):
    """地标类别"""
    SUBWAY = "subway"
    COMPANY = "company"
    LANDMARK = "landmark"


@dataclass
class Landmark:
    """地标数据模型"""
    id: str
    name: str
    category: LandmarkCategory
    district: str
    longitude: float
    latitude: float
    raw_data: Dict

    def to_dict(self) -> Dict:
        """转换为字典"""
        return {
            "id": self.id,
            "name": self.name,
            "category": self.category.value,
            "district": self.district,
            "longitude": self.longitude,
            "latitude": self.latitude,
            "details": self.raw_data
        }


class LandmarkManager:
    """地标数据管理器"""

    def __init__(self, data_dir: str = None):
        """
        初始化地标管理器

        Args:
            data_dir: 数据目录路径，默认为当前文件所在目录下的data文件夹
        """
        if data_dir is None:
            data_dir = os.path.join(os.path.dirname(__file__), "data")
        self.data_dir = data_dir
        self.landmarks: Dict[str, Landmark] = {}
        self._load_all_data()

    def _load_json(self, filename: str) -> Dict:
        """加载JSON文件"""
        filepath = os.path.join(self.data_dir, filename)
        try:
            with open(filepath, 'r', encoding='utf-8') as f:
                return json.load(f)
        except FileNotFoundError:
            raise FileNotFoundError(f"数据文件不存在: {filepath}")
        except json.JSONDecodeError as e:
            raise ValueError(f"JSON解析错误: {filepath}, 错误: {e}")

    def _load_subway_stations(self):
        """加载地铁站数据"""
        data = self._load_json("subway_stations.json")
        for station in data.get("stations", []):
            landmark = Landmark(
                id=station["station_id"],
                name=station["name"],
                category=LandmarkCategory.SUBWAY,
                district=station["district"],
                longitude=station["longitude"],
                latitude=station["latitude"],
                raw_data=station
            )
            self.landmarks[landmark.id] = landmark

    def _load_companies(self):
        """加载世界500强企业数据"""
        data = self._load_json("fortune500_companies.json")
        for company in data.get("companies", []):
            landmark = Landmark(
                id=company["company_id"],
                name=company["name"],
                category=LandmarkCategory.COMPANY,
                district=company["district"],
                longitude=company["longitude"],
                latitude=company["latitude"],
                raw_data=company
            )
            self.landmarks[landmark.id] = landmark

    def _load_landmarks(self):
        """加载商圈地标数据"""
        data = self._load_json("landmarks.json")
        for lm in data.get("landmarks", []):
            landmark = Landmark(
                id=lm["landmark_id"],
                name=lm["name"],
                category=LandmarkCategory.LANDMARK,
                district=lm["district"],
                longitude=lm["longitude"],
                latitude=lm["latitude"],
                raw_data=lm
            )
            self.landmarks[landmark.id] = landmark

    def _load_all_data(self):
        """加载所有地标数据"""
        self._load_subway_stations()
        self._load_companies()
        self._load_landmarks()

    def get_by_name(self, name: str, category: Optional[LandmarkCategory] = None) -> Optional[Landmark]:
        """
        根据名称查询地标（精确匹配）

        Args:
            name: 地标名称
            category: 可选，限定类别

        Returns:
            匹配的地标对象，未找到返回None
        """
        for landmark in self.landmarks.values():
            if landmark.name == name:
                if category is None or landmark.category == category:
                    return landmark
        return None

    def search_by_name(self, keyword: str, category: Optional[LandmarkCategory] = None) -> List[Landmark]:
        """
        根据关键词搜索地标（模糊匹配）

        Args:
            keyword: 搜索关键词
            category: 可选，限定类别

        Returns:
            匹配的地标列表
        """
        results = []
        keyword_lower = keyword.lower()

        for landmark in self.landmarks.values():
            # 检查名称是否包含关键词
            if keyword_lower in landmark.name.lower():
                if category is None or landmark.category == category:
                    results.append(landmark)
                    continue

            # 检查别名（针对企业）
            if landmark.category == LandmarkCategory.COMPANY:
                short_name = landmark.raw_data.get("short_name", "")
                name_en = landmark.raw_data.get("name_en", "")
                if (keyword_lower in short_name.lower() or
                    keyword_lower in name_en.lower()):
                    results.append(landmark)

        return results

    def get_all(self, category: Optional[LandmarkCategory] = None) -> List[Landmark]:
        """
        获取全部地标信息

        Args:
            category: 可选，按类别筛选

        Returns:
            地标列表
        """
        if category is None:
            return list(self.landmarks.values())
        return [lm for lm in self.landmarks.values() if lm.category == category]

    def get_by_category(self, category: LandmarkCategory) -> List[Landmark]:
        """
        按类别获取地标

        Args:
            category: 地标类别

        Returns:
            该类别下的所有地标
        """
        return self.get_all(category=category)

    def get_by_district(self, district: str) -> List[Landmark]:
        """
        按行政区获取地标

        Args:
            district: 行政区名称

        Returns:
            该行政区内的所有地标
        """
        return [lm for lm in self.landmarks.values() if lm.district == district]

    def get_by_id(self, landmark_id: str) -> Optional[Landmark]:
        """
        根据ID获取地标

        Args:
            landmark_id: 地标ID

        Returns:
            地标对象，未找到返回None
        """
        return self.landmarks.get(landmark_id)

    def get_statistics(self) -> Dict:
        """
        获取地标数据统计信息

        Returns:
            统计信息字典
        """
        stats = {
            "total": len(self.landmarks),
            "by_category": {
                "subway": len(self.get_by_category(LandmarkCategory.SUBWAY)),
                "company": len(self.get_by_category(LandmarkCategory.COMPANY)),
                "landmark": len(self.get_by_category(LandmarkCategory.LANDMARK))
            },
            "by_district": {}
        }

        # 按行政区统计
        for landmark in self.landmarks.values():
            district = landmark.district
            if district not in stats["by_district"]:
                stats["by_district"][district] = 0
            stats["by_district"][district] += 1

        return stats


# 便捷函数接口

_manager = None


def get_manager() -> LandmarkManager:
    """获取全局地标管理器实例（单例模式）"""
    global _manager
    if _manager is None:
        _manager = LandmarkManager()
    return _manager


def query_by_name(name: str, category: Optional[str] = None) -> Optional[Dict]:
    """
    根据名称查询地标（便捷接口）

    Args:
        name: 地标名称
        category: 类别，可选值：subway/company/landmark

    Returns:
        地标信息字典，未找到返回None
    """
    cat = None
    if category:
        cat = LandmarkCategory(category)

    manager = get_manager()
    landmark = manager.get_by_name(name, cat)
    return landmark.to_dict() if landmark else None


def search_by_keyword(keyword: str, category: Optional[str] = None) -> List[Dict]:
    """
    根据关键词搜索地标（便捷接口）

    Args:
        keyword: 搜索关键词
        category: 类别，可选值：subway/company/landmark

    Returns:
        地标信息字典列表
    """
    cat = None
    if category:
        cat = LandmarkCategory(category)

    manager = get_manager()
    landmarks = manager.search_by_name(keyword, cat)
    return [lm.to_dict() for lm in landmarks]


def query_all(category: Optional[str] = None) -> List[Dict]:
    """
    查询全部地标信息（便捷接口）

    Args:
        category: 类别，可选值：subway/company/landmark，不传返回全部

    Returns:
        地标信息字典列表
    """
    cat = None
    if category:
        cat = LandmarkCategory(category)

    manager = get_manager()
    landmarks = manager.get_all(cat)
    return [lm.to_dict() for lm in landmarks]


def get_statistics() -> Dict:
    """获取地标数据统计信息（便捷接口）"""
    manager = get_manager()
    return manager.get_statistics()


if __name__ == "__main__":
    # 测试代码
    print("=" * 50)
    print("地标数据管理器测试")
    print("=" * 50)

    # 测试1: 获取统计信息
    print("\n【测试1】统计信息:")
    stats = get_statistics()
    print(f"总数量: {stats['total']}")
    print(f"按类别: {stats['by_category']}")
    print(f"按行政区: {stats['by_district']}")

    # 测试2: 根据名称精确查询
    print("\n【测试2】精确查询'西二旗站':")
    result = query_by_name("西二旗站")
    if result:
        print(f"找到: {result['name']} ({result['category']})")
        print(f"坐标: ({result['longitude']}, {result['latitude']})")
        print(f"行政区: {result['district']}")

    # 测试3: 根据关键词模糊搜索
    print("\n【测试3】模糊搜索'百度':")
    results = search_by_keyword("百度")
    for r in results:
        print(f"  - {r['name']} ({r['category']})")

    # 测试4: 查询全部地铁站
    print("\n【测试4】查询全部地铁站(前5个):")
    subways = query_all("subway")
    for s in subways[:5]:
        print(f"  - {s['name']}: {s['details']['lines']}")

    # 测试5: 查询全部企业
    print("\n【测试5】查询全部企业(前5个):")
    companies = query_all("company")
    for c in companies[:5]:
        print(f"  - {c['name']} ({c['details']['short_name']})")

    # 测试6: 查询全部地标
    print("\n【测试6】查询全部地标(前5个):")
    landmarks = query_all("landmark")
    for l in landmarks[:5]:
        print(f"  - {l['name']} ({l['details']['type_name']})")

    # 测试7: 查询全部数据
    print(f"\n【测试7】查询全部地标数据: 共{len(query_all())}条")

    print("\n" + "=" * 50)
    print("测试完成")
    print("=" * 50)
