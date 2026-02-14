/**
 * UPnP Device and Service Description XML Generation
 * Generates XML documents required by UPnP specification
 */

import { createLogger } from '@nself/plugin-utils';
import type { DeviceDescription, ServiceDescription } from './types.js';

const logger = createLogger('dlna:upnp');

/**
 * Default device description values
 */
const DEFAULT_DEVICE: DeviceDescription = {
  uuid: '',
  friendlyName: 'nself-tv Media Server',
  manufacturer: 'nself',
  manufacturerURL: 'https://nself.org',
  modelName: 'nself DLNA Media Server',
  modelNumber: '1.0',
  modelDescription: 'DLNA/UPnP Media Server for nself-tv',
  modelURL: 'https://github.com/acamarata/nself-plugins',
  serialNumber: '1',
  presentationURL: '',
};

/**
 * Service definitions for the MediaServer device
 */
const SERVICES: ServiceDescription[] = [
  {
    serviceType: 'urn:schemas-upnp-org:service:ContentDirectory:1',
    serviceId: 'urn:upnp-org:serviceId:ContentDirectory',
    SCPDURL: '/ContentDirectory.xml',
    controlURL: '/control/ContentDirectory',
    eventSubURL: '/event/ContentDirectory',
  },
  {
    serviceType: 'urn:schemas-upnp-org:service:ConnectionManager:1',
    serviceId: 'urn:upnp-org:serviceId:ConnectionManager',
    SCPDURL: '/ConnectionManager.xml',
    controlURL: '/control/ConnectionManager',
    eventSubURL: '/event/ConnectionManager',
  },
];

/**
 * Escape a string for safe XML inclusion
 */
function escapeXml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

/**
 * Generate the UPnP device description XML document
 * This is served at GET /description.xml
 */
export function generateDeviceDescription(
  uuid: string,
  friendlyName: string,
  httpPort: number,
  httpHost: string
): string {
  const device: DeviceDescription = {
    ...DEFAULT_DEVICE,
    uuid,
    friendlyName,
    presentationURL: `http://${httpHost}:${httpPort}/`,
  };

  const serviceList = SERVICES.map(svc => `
        <service>
          <serviceType>${escapeXml(svc.serviceType)}</serviceType>
          <serviceId>${escapeXml(svc.serviceId)}</serviceId>
          <SCPDURL>${escapeXml(svc.SCPDURL)}</SCPDURL>
          <controlURL>${escapeXml(svc.controlURL)}</controlURL>
          <eventSubURL>${escapeXml(svc.eventSubURL)}</eventSubURL>
        </service>`).join('\n');

  logger.debug('Generated device description', { uuid, friendlyName });

  return `<?xml version="1.0" encoding="UTF-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0"
      xmlns:dlna="urn:schemas-dlna-org:device-1-0">
  <specVersion>
    <major>1</major>
    <minor>1</minor>
  </specVersion>
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
    <friendlyName>${escapeXml(device.friendlyName)}</friendlyName>
    <manufacturer>${escapeXml(device.manufacturer)}</manufacturer>
    <manufacturerURL>${escapeXml(device.manufacturerURL)}</manufacturerURL>
    <modelDescription>${escapeXml(device.modelDescription)}</modelDescription>
    <modelName>${escapeXml(device.modelName)}</modelName>
    <modelNumber>${escapeXml(device.modelNumber)}</modelNumber>
    <modelURL>${escapeXml(device.modelURL)}</modelURL>
    <serialNumber>${escapeXml(device.serialNumber)}</serialNumber>
    <UDN>uuid:${escapeXml(device.uuid)}</UDN>
    <presentationURL>${escapeXml(device.presentationURL)}</presentationURL>
    <dlna:X_DLNADOC xmlns:dlna="urn:schemas-dlna-org:device-1-0">DMS-1.50</dlna:X_DLNADOC>
    <serviceList>${serviceList}
    </serviceList>
  </device>
</root>`;
}

/**
 * Generate the ContentDirectory service description (SCPD) XML
 * This is served at GET /ContentDirectory.xml
 */
export function generateContentDirectorySCPD(): string {
  return `<?xml version="1.0" encoding="UTF-8"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion>
    <major>1</major>
    <minor>0</minor>
  </specVersion>
  <actionList>
    <action>
      <name>Browse</name>
      <argumentList>
        <argument>
          <name>ObjectID</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_ObjectID</relatedStateVariable>
        </argument>
        <argument>
          <name>BrowseFlag</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_BrowseFlag</relatedStateVariable>
        </argument>
        <argument>
          <name>Filter</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_Filter</relatedStateVariable>
        </argument>
        <argument>
          <name>StartingIndex</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_Index</relatedStateVariable>
        </argument>
        <argument>
          <name>RequestedCount</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable>
        </argument>
        <argument>
          <name>SortCriteria</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_SortCriteria</relatedStateVariable>
        </argument>
        <argument>
          <name>Result</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_Result</relatedStateVariable>
        </argument>
        <argument>
          <name>NumberReturned</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable>
        </argument>
        <argument>
          <name>TotalMatches</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable>
        </argument>
        <argument>
          <name>UpdateID</name>
          <direction>out</direction>
          <relatedStateVariable>SystemUpdateID</relatedStateVariable>
        </argument>
      </argumentList>
    </action>
    <action>
      <name>Search</name>
      <argumentList>
        <argument>
          <name>ContainerID</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_ObjectID</relatedStateVariable>
        </argument>
        <argument>
          <name>SearchCriteria</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_SearchCriteria</relatedStateVariable>
        </argument>
        <argument>
          <name>Filter</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_Filter</relatedStateVariable>
        </argument>
        <argument>
          <name>StartingIndex</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_Index</relatedStateVariable>
        </argument>
        <argument>
          <name>RequestedCount</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable>
        </argument>
        <argument>
          <name>SortCriteria</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_SortCriteria</relatedStateVariable>
        </argument>
        <argument>
          <name>Result</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_Result</relatedStateVariable>
        </argument>
        <argument>
          <name>NumberReturned</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable>
        </argument>
        <argument>
          <name>TotalMatches</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable>
        </argument>
        <argument>
          <name>UpdateID</name>
          <direction>out</direction>
          <relatedStateVariable>SystemUpdateID</relatedStateVariable>
        </argument>
      </argumentList>
    </action>
    <action>
      <name>GetSystemUpdateID</name>
      <argumentList>
        <argument>
          <name>Id</name>
          <direction>out</direction>
          <relatedStateVariable>SystemUpdateID</relatedStateVariable>
        </argument>
      </argumentList>
    </action>
    <action>
      <name>GetSearchCapabilities</name>
      <argumentList>
        <argument>
          <name>SearchCaps</name>
          <direction>out</direction>
          <relatedStateVariable>SearchCapabilities</relatedStateVariable>
        </argument>
      </argumentList>
    </action>
    <action>
      <name>GetSortCapabilities</name>
      <argumentList>
        <argument>
          <name>SortCaps</name>
          <direction>out</direction>
          <relatedStateVariable>SortCapabilities</relatedStateVariable>
        </argument>
      </argumentList>
    </action>
  </actionList>
  <serviceStateTable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_ObjectID</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_Result</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_BrowseFlag</name>
      <dataType>string</dataType>
      <allowedValueList>
        <allowedValue>BrowseMetadata</allowedValue>
        <allowedValue>BrowseDirectChildren</allowedValue>
      </allowedValueList>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_Filter</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_SortCriteria</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_SearchCriteria</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_Index</name>
      <dataType>ui4</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_Count</name>
      <dataType>ui4</dataType>
    </stateVariable>
    <stateVariable sendEvents="yes">
      <name>SystemUpdateID</name>
      <dataType>ui4</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>SearchCapabilities</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>SortCapabilities</name>
      <dataType>string</dataType>
    </stateVariable>
  </serviceStateTable>
</scpd>`;
}

/**
 * Generate the ConnectionManager service description (SCPD) XML
 * This is served at GET /ConnectionManager.xml
 */
export function generateConnectionManagerSCPD(): string {
  return `<?xml version="1.0" encoding="UTF-8"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion>
    <major>1</major>
    <minor>0</minor>
  </specVersion>
  <actionList>
    <action>
      <name>GetProtocolInfo</name>
      <argumentList>
        <argument>
          <name>Source</name>
          <direction>out</direction>
          <relatedStateVariable>SourceProtocolInfo</relatedStateVariable>
        </argument>
        <argument>
          <name>Sink</name>
          <direction>out</direction>
          <relatedStateVariable>SinkProtocolInfo</relatedStateVariable>
        </argument>
      </argumentList>
    </action>
    <action>
      <name>GetCurrentConnectionIDs</name>
      <argumentList>
        <argument>
          <name>ConnectionIDs</name>
          <direction>out</direction>
          <relatedStateVariable>CurrentConnectionIDs</relatedStateVariable>
        </argument>
      </argumentList>
    </action>
    <action>
      <name>GetCurrentConnectionInfo</name>
      <argumentList>
        <argument>
          <name>ConnectionID</name>
          <direction>in</direction>
          <relatedStateVariable>A_ARG_TYPE_ConnectionID</relatedStateVariable>
        </argument>
        <argument>
          <name>RcsID</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_RcsID</relatedStateVariable>
        </argument>
        <argument>
          <name>AVTransportID</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_AVTransportID</relatedStateVariable>
        </argument>
        <argument>
          <name>ProtocolInfo</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_ProtocolInfo</relatedStateVariable>
        </argument>
        <argument>
          <name>PeerConnectionManager</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_ConnectionManager</relatedStateVariable>
        </argument>
        <argument>
          <name>PeerConnectionID</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_ConnectionID</relatedStateVariable>
        </argument>
        <argument>
          <name>Direction</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_Direction</relatedStateVariable>
        </argument>
        <argument>
          <name>Status</name>
          <direction>out</direction>
          <relatedStateVariable>A_ARG_TYPE_ConnectionStatus</relatedStateVariable>
        </argument>
      </argumentList>
    </action>
  </actionList>
  <serviceStateTable>
    <stateVariable sendEvents="yes">
      <name>SourceProtocolInfo</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="yes">
      <name>SinkProtocolInfo</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="yes">
      <name>CurrentConnectionIDs</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_ConnectionStatus</name>
      <dataType>string</dataType>
      <allowedValueList>
        <allowedValue>OK</allowedValue>
        <allowedValue>ContentFormatMismatch</allowedValue>
        <allowedValue>InsufficientBandwidth</allowedValue>
        <allowedValue>UnreliableChannel</allowedValue>
        <allowedValue>Unknown</allowedValue>
      </allowedValueList>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_ConnectionManager</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_Direction</name>
      <dataType>string</dataType>
      <allowedValueList>
        <allowedValue>Input</allowedValue>
        <allowedValue>Output</allowedValue>
      </allowedValueList>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_ProtocolInfo</name>
      <dataType>string</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_ConnectionID</name>
      <dataType>i4</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_AVTransportID</name>
      <dataType>i4</dataType>
    </stateVariable>
    <stateVariable sendEvents="no">
      <name>A_ARG_TYPE_RcsID</name>
      <dataType>i4</dataType>
    </stateVariable>
  </serviceStateTable>
</scpd>`;
}

/**
 * Get the list of supported source protocol info strings
 * Used by ConnectionManager GetProtocolInfo action
 */
export function getSupportedProtocolInfo(): string {
  const protocols = [
    'http-get:*:video/mp4:*',
    'http-get:*:video/x-matroska:*',
    'http-get:*:video/x-msvideo:*',
    'http-get:*:video/quicktime:*',
    'http-get:*:video/x-ms-wmv:*',
    'http-get:*:video/webm:*',
    'http-get:*:video/mpeg:*',
    'http-get:*:video/mp2t:*',
    'http-get:*:video/3gpp:*',
    'http-get:*:video/ogg:*',
    'http-get:*:audio/mpeg:*',
    'http-get:*:audio/flac:*',
    'http-get:*:audio/wav:*',
    'http-get:*:audio/aac:*',
    'http-get:*:audio/mp4:*',
    'http-get:*:audio/ogg:*',
    'http-get:*:audio/x-ms-wma:*',
    'http-get:*:audio/opus:*',
    'http-get:*:image/jpeg:*',
    'http-get:*:image/png:*',
    'http-get:*:image/gif:*',
    'http-get:*:image/bmp:*',
    'http-get:*:image/webp:*',
  ];

  return protocols.join(',');
}
