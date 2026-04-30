from __future__ import annotations

import enum
import sys
import types
from datetime import datetime


def install() -> None:
    if "homeassistant" in sys.modules:
        return

    aiohttp = types.ModuleType("aiohttp")
    aiohttp.ClientError = Exception
    aiohttp.ClientSession = object
    sys.modules["aiohttp"] = aiohttp

    voluptuous = types.ModuleType("voluptuous")

    def optional(key):
        return key

    voluptuous.Optional = optional
    sys.modules["voluptuous"] = voluptuous

    homeassistant = types.ModuleType("homeassistant")
    sys.modules["homeassistant"] = homeassistant

    components = types.ModuleType("homeassistant.components")
    sys.modules["homeassistant.components"] = components

    class CameraEntityFeature(enum.IntFlag):
        STREAM = 1

    class Camera:
        def __init__(self) -> None:
            self.hass = None
            self.entity_id = None

    camera = types.ModuleType("homeassistant.components.camera")
    camera.Camera = Camera
    camera.CameraEntityFeature = CameraEntityFeature
    sys.modules["homeassistant.components.camera"] = camera

    class BinarySensorDeviceClass(str, enum.Enum):
        RUNNING = "running"
        PROBLEM = "problem"
        MOTION = "motion"
        CONNECTIVITY = "connectivity"
        TAMPER = "tamper"

    binary_sensor = types.ModuleType("homeassistant.components.binary_sensor")
    binary_sensor.BinarySensorDeviceClass = BinarySensorDeviceClass
    sys.modules["homeassistant.components.binary_sensor"] = binary_sensor

    class SensorDeviceClass(str, enum.Enum):
        TIMESTAMP = "timestamp"

    sensor = types.ModuleType("homeassistant.components.sensor")
    sensor.SensorDeviceClass = SensorDeviceClass
    sys.modules["homeassistant.components.sensor"] = sensor

    class Platform(str, enum.Enum):
        CAMERA = "camera"
        BINARY_SENSOR = "binary_sensor"
        SENSOR = "sensor"
        BUTTON = "button"
        NUMBER = "number"
        SWITCH = "switch"

    class EntityCategory(str, enum.Enum):
        DIAGNOSTIC = "diagnostic"

    ha_const = types.ModuleType("homeassistant.const")
    ha_const.Platform = Platform
    ha_const.EntityCategory = EntityCategory
    sys.modules["homeassistant.const"] = ha_const

    class ConfigEntry:
        def __init__(self, options=None) -> None:
            self.options = options or {}

    config_entries = types.ModuleType("homeassistant.config_entries")
    config_entries.ConfigEntry = ConfigEntry
    sys.modules["homeassistant.config_entries"] = config_entries

    class HomeAssistant:
        pass

    def callback(func):
        return func

    core = types.ModuleType("homeassistant.core")
    core.HomeAssistant = HomeAssistant
    core.callback = callback
    sys.modules["homeassistant.core"] = core

    class HomeAssistantError(Exception):
        pass

    exceptions = types.ModuleType("homeassistant.exceptions")
    exceptions.HomeAssistantError = HomeAssistantError
    sys.modules["homeassistant.exceptions"] = exceptions

    helpers = types.ModuleType("homeassistant.helpers")
    sys.modules["homeassistant.helpers"] = helpers

    aiohttp_client = types.ModuleType("homeassistant.helpers.aiohttp_client")
    aiohttp_client.async_get_clientsession = lambda hass: object()
    sys.modules["homeassistant.helpers.aiohttp_client"] = aiohttp_client

    config_validation = types.ModuleType("homeassistant.helpers.config_validation")
    config_validation.string = str
    config_validation.positive_int = int
    sys.modules["homeassistant.helpers.config_validation"] = config_validation

    class DeviceInfo(dict):
        pass

    device_registry = types.ModuleType("homeassistant.helpers.device_registry")
    device_registry.DeviceInfo = DeviceInfo
    sys.modules["homeassistant.helpers.device_registry"] = device_registry

    class CoordinatorEntity:
        @classmethod
        def __class_getitem__(cls, item):
            return cls

        def __init__(self, coordinator) -> None:
            self.coordinator = coordinator

        @property
        def available(self) -> bool:
            return True

    class DataUpdateCoordinator:
        @classmethod
        def __class_getitem__(cls, item):
            return cls

        def __init__(self, *args, **kwargs) -> None:
            self.data = None
            self.last_update_success = True

    class UpdateFailed(Exception):
        pass

    update_coordinator = types.ModuleType("homeassistant.helpers.update_coordinator")
    update_coordinator.CoordinatorEntity = CoordinatorEntity
    update_coordinator.DataUpdateCoordinator = DataUpdateCoordinator
    update_coordinator.UpdateFailed = UpdateFailed
    sys.modules["homeassistant.helpers.update_coordinator"] = update_coordinator

    class _DummyPlatform:
        def async_register_entity_service(self, *args, **kwargs) -> None:
            return None

    entity_platform = types.ModuleType("homeassistant.helpers.entity_platform")
    entity_platform.AddEntitiesCallback = object
    entity_platform.async_get_current_platform = lambda: _DummyPlatform()
    sys.modules["homeassistant.helpers.entity_platform"] = entity_platform

    util = types.ModuleType("homeassistant.util")
    sys.modules["homeassistant.util"] = util

    dt = types.ModuleType("homeassistant.util.dt")
    dt.parse_datetime = lambda value: datetime.fromisoformat(value.replace("Z", "+00:00"))
    sys.modules["homeassistant.util.dt"] = dt
