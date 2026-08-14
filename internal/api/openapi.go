package api

// openAPISpec is the OpenAPI 3.0 specification for the camspeak REST API.
// Served at /api/openapi.json and used by the Swagger UI at /swagger.
const openAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "camspeak API",
    "description": "Camera audio router — stream TTS and audio to IP camera speakers via Hikvision ISAPI, Reolink, go2rtc, or ONVIF RTSP backchannel. First-class Home Assistant integration available via HACS (https://github.com/jeeftor/camspeak-hacs).",
    "version": "1.0",
    "license": {
      "name": "MIT",
      "url": "https://github.com/jeeftor/camspeak/blob/master/LICENSE"
    }
  },
  "servers": [
    {"url": "/api", "description": "Relative to this server"}
  ],
  "tags": [
    {"name": "audio", "description": "Speak, play, beep, broadcast"},
    {"name": "vision", "description": "Snapshot, vision, describe"},
    {"name": "library", "description": "Preset management"},
    {"name": "config", "description": "Runtime configuration"},
    {"name": "mqtt", "description": "MQTT status and browser"},
    {"name": "system", "description": "Health, events, cameras"}
  ],
  "paths": {
    "/speak": {
      "post": {
        "tags": ["audio"],
        "summary": "Text-to-speech on a single camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/SpeakRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/StatusResponse"}}}},
          "503": {"description": "TTS not configured"}
        }
      }
    },
    "/broadcast": {
      "post": {
        "tags": ["audio"],
        "summary": "TTS or preset to all cameras simultaneously",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/BroadcastRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/play": {
      "post": {
        "tags": ["audio"],
        "summary": "Play a saved library preset on a camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/PlayRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK"},
          "404": {"description": "Preset not found"}
        }
      }
    },
    "/play-url": {
      "post": {
        "tags": ["audio"],
        "summary": "Download audio from URL, transcode, and play on camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/PlayURLRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/play-stream": {
      "post": {
        "tags": ["audio"],
        "summary": "Stream live audio from a URL or playlist to a camera",
        "description": "Starts an ffmpeg process that reads a live stream or playlist (.pls/.m3u) and sends raw G.711 mu-law to the camera speaker. Currently supported for cameras with a real Stream implementation (e.g. Hikvision). Stop with POST /api/stop.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/PlayURLRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/beep": {
      "post": {
        "tags": ["audio"],
        "summary": "Play an 800 Hz test beep on a camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/CameraRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/stop": {
      "post": {
        "tags": ["audio"],
        "summary": "Stop audio playback on a specific camera or all cameras",
        "description": "If the request body contains a camera name, only that camera is stopped. If empty or omitted, all cameras are stopped. Tears down the ffmpeg process, closes the camera speaker connection, and resets AirPlay. For a softer suspend that can be resumed in place, use POST /api/pause instead.",
        "requestBody": {
          "required": false,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "camera": {"type": "string", "description": "Camera name to stop. If omitted, stops all cameras."}
                }
              }
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "object", "properties": {"status": {"type": "string"}, "camera": {"type": "string"}}}}}},
          "404": {"description": "Camera not found"}
        }
      }
    },
    "/pause": {
      "post": {
        "tags": ["audio"],
        "summary": "Pause a live stream on a specific camera or all cameras",
        "description": "Suspends the ffmpeg transcoder for an active /api/play-stream session or a looped preset (/api/play with loop=true) via SIGSTOP without tearing down the camera speaker connection. Playback position is preserved and can be resumed in place with POST /api/resume. Only affects streams and looped presets; finite TTS/play/beep sends are unaffected. If the camera is omitted, all active streams are paused.",
        "requestBody": {
          "required": false,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "camera": {"type": "string", "description": "Camera name to pause. If omitted, pauses all active streams."}
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": {"type": "string", "description": "paused, already-paused, or (when omitting camera) paused with a cameras array"},
                    "camera": {"type": "string"},
                    "cameras": {"type": "array", "items": {"type": "string"}}
                  }
                }
              }
            }
          },
          "404": {"description": "No active stream for the named camera"}
        }
      }
    },
    "/resume": {
      "post": {
        "tags": ["audio"],
        "summary": "Resume a paused live stream on a specific camera or all cameras",
        "description": "Resumes a stream or looped preset previously paused with POST /api/pause by sending SIGCONT to the ffmpeg transcoder. Only affects streams started via /api/play-stream and looped presets started via /api/play with loop=true. If the camera is omitted, all paused streams are resumed.",
        "requestBody": {
          "required": false,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "camera": {"type": "string", "description": "Camera name to resume. If omitted, resumes all paused streams."}
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": {"type": "string", "description": "resumed, not-paused, or (when omitting camera) resumed with a cameras array"},
                    "camera": {"type": "string"},
                    "cameras": {"type": "array", "items": {"type": "string"}}
                  }
                }
              }
            }
          },
          "404": {"description": "No active stream for the named camera"}
        }
      }
    },
    "/snapshot/{camera}": {
      "get": {
        "tags": ["vision"],
        "summary": "Fetch a JPEG snapshot from the camera",
        "parameters": [
          {"name": "camera", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "JPEG image", "content": {"image/jpeg": {"schema": {"type": "string", "format": "binary"}}}},
          "502": {"description": "Frigate not reachable"}
        }
      }
    },
    "/vision": {
      "post": {
        "tags": ["vision"],
        "summary": "Snapshot to vision model, returns text description only (no TTS)",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/VisionRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionResponse"}}}},
          "503": {"description": "Vision model not configured"}
        }
      }
    },
    "/vision/test": {
      "post": {
        "tags": ["vision"],
        "summary": "Capture snapshot (or reuse provided image) and run a vision prompt — for prompt testing/refinement",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/VisionTestRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionTestResponse"}}}},
          "503": {"description": "Vision model or Frigate not configured"}
        }
      }
    },
    "/describe": {
      "post": {
        "tags": ["vision"],
        "summary": "Snapshot to vision model to TTS to speak on camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/DescribeRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/DescribeResponse"}}}},
          "503": {"description": "Vision or TTS not configured"}
        }
      }
    },
    "/cameras": {
      "get": {
        "tags": ["system"],
        "summary": "List all cameras with online status",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Camera"}}}}}
        }
      }
    },
    "/playback": {
      "get": {
        "tags": ["audio"],
        "summary": "Get current playback state for all cameras",
        "description": "Returns a map of camera name to its current audio playback state. Each entry has a state field (\"playing\", \"paused\", or \"idle\"), a source field (\"stream\", \"speak\", \"play\", \"play-url\", \"beep\"), a detail string (the stream URL, TTS text, preset name, etc.), and timestamps for when playback started and when it was paused.",
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "additionalProperties": {"$ref": "#/components/schemas/PlaybackState"}
                }
              }
            }
          }
        }
      }
    },
    "/cameras/{name}/info": {
      "get": {
        "tags": ["system"],
        "summary": "Query camera device info and streaming settings (ISAPI/ONVIF)",
        "description": "Queries the camera's vendor API (Hikvision ISAPI or ONVIF SOAP) and returns device info, video/audio encoder configuration, and network info. Read-only.",
        "parameters": [
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/CameraInfo"}}}},
          "404": {"description": "Camera not found"},
          "502": {"description": "Camera unreachable or query failed"}
        }
      }
    },
    "/voices": {
      "get": {
        "tags": ["system"],
        "summary": "List available TTS voices",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "array", "items": {"type": "string"}}}}}
        }
      }
    },
    "/library": {
      "get": {
        "tags": ["library"],
        "summary": "List all saved audio presets",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Preset"}}}}}
        }
      },
      "post": {
        "tags": ["library"],
        "summary": "Generate a TTS clip and save as a preset",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/GeneratePresetRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Preset"}}}}
        }
      }
    },
    "/library/upload": {
      "post": {
        "tags": ["library"],
        "summary": "Upload an audio file (any format, ffmpeg transcodes to G.711)",
        "description": "Accepts a multipart upload, saves the temp file, and starts ffmpeg transcoding in the background. Returns a job_id immediately — poll GET /api/library/upload/jobs/{job_id} for transcoding progress.",
        "requestBody": {
          "required": true,
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "properties": {
                  "name": {"type": "string"},
                  "category": {"type": "string"},
                  "file": {"type": "string", "format": "binary"}
                },
                "required": ["name", "file"]
              }
            }
          }
        },
        "responses": {
          "202": {"description": "Accepted — transcoding started", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/UploadJobAccepted"}}}}
        }
      }
    },
    "/library/upload/jobs/{id}": {
      "get": {
        "tags": ["library"],
        "summary": "Poll the status of an async upload/transcode job",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/UploadJob"}}}},
          "404": {"description": "Job not found"}
        }
      }
    },
    "/library/{category}/{name}": {
      "delete": {
        "tags": ["library"],
        "summary": "Delete a library preset",
        "parameters": [
          {"name": "category", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "OK"}
        }
      },
      "patch": {
        "tags": ["library"],
        "summary": "Rename a preset (change name and/or category)",
        "parameters": [
          {"name": "category", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}, "category": {"type": "string"}}}}}},
        "responses": {
          "200": {"description": "Updated preset"},
          "409": {"description": "Target name already exists"}
        }
      }
    },
    "/library/{category}/{name}/preview": {
      "get": {
        "tags": ["library"],
        "summary": "Stream the audio for a preset (for in-browser playback)",
        "parameters": [
          {"name": "category", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "Audio stream", "content": {"audio/*": {"schema": {"type": "string", "format": "binary"}}}}
        }
      }
    },
    "/tts/preview": {
      "post": {
        "tags": ["library"],
        "summary": "Generate a TTS preview (audio blob, not saved)",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/TTSPreviewRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "Audio blob", "content": {"audio/wav": {"schema": {"type": "string", "format": "binary"}}}}
        }
      }
    },
    "/events": {
      "get": {
        "tags": ["system"],
        "summary": "Server-Sent Events stream of speak/play/beep/broadcast/describe/stop actions",
        "responses": {
          "200": {"description": "SSE stream", "content": {"text/event-stream": {"schema": {"type": "string"}}}}
        }
      }
    },
    "/events/log": {
      "get": {
        "tags": ["system"],
        "summary": "Query historical event log as JSON",
        "parameters": [
          {"name": "limit", "in": "query", "schema": {"type": "integer", "default": 100, "maximum": 1000}, "description": "Max events to return"},
          {"name": "camera", "in": "query", "schema": {"type": "string"}, "description": "Filter by camera name"}
        ],
        "responses": {
          "200": {"description": "Event log", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/EventEntry"}}}}}
        }
      }
    },
    "/health": {
      "get": {
        "tags": ["system"],
        "summary": "Health check with version",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/HealthResponse"}}}}
        }
      }
    },
    "/config": {
      "get": {
        "tags": ["config"],
        "summary": "Current runtime configuration",
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/config/vision": {
      "get": {
        "tags": ["config"],
        "summary": "Get vision endpoint config",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionConfig"}}}}
        }
      },
      "put": {
        "tags": ["config"],
        "summary": "Update vision endpoint config (rebuilds vision client at runtime)",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/VisionConfig"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionConfig"}}}}
        }
      }
    },
    "/config/vision-prompts": {
      "get": {
        "tags": ["config"],
        "summary": "List all saved vision prompt presets",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/VisionPromptPreset"}}}}}
        }
      },
      "post": {
        "tags": ["config"],
        "summary": "Create or update a vision prompt preset",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/VisionPromptPreset"}
            }
          }
        },
        "responses": {
          "201": {"description": "Created", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionPromptPreset"}}}}
        }
      }
    },
    "/config/vision-prompts/{name}": {
      "delete": {
        "tags": ["config"],
        "summary": "Delete a vision prompt preset",
        "parameters": [
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/config/tts": {
      "get": {
        "tags": ["config"],
        "summary": "List all TTS presets",
        "responses": {
          "200": {"description": "OK"}
        }
      },
      "post": {
        "tags": ["config"],
        "summary": "Create a TTS preset",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TTSPreset"}}}},
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/tts/{name}": {
      "put": {
        "tags": ["config"],
        "summary": "Update a TTS preset",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TTSPreset"}}}},
        "responses": {"200": {"description": "OK"}}
      },
      "delete": {
        "tags": ["config"],
        "summary": "Delete a TTS preset (not the active one)",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}, "409": {"description": "Cannot delete active preset"}}
      }
    },
    "/config/tts/{name}/activate": {
      "post": {
        "tags": ["config"],
        "summary": "Set a TTS preset as active",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/cameras": {
      "get": {
        "tags": ["config"],
        "summary": "List cameras from config",
        "responses": {"200": {"description": "OK"}}
      },
      "post": {
        "tags": ["config"],
        "summary": "Add a camera",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/CameraConfig"}}}},
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/cameras/{name}/toggle": {
      "patch": {
        "tags": ["config"],
        "summary": "Toggle camera enabled/disabled",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/cameras/{name}": {
      "delete": {
        "tags": ["config"],
        "summary": "Remove a camera",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/rules": {
      "get": {
        "tags": ["config"],
        "summary": "List MQTT rules",
        "responses": {"200": {"description": "OK"}}
      },
      "post": {
        "tags": ["config"],
        "summary": "Create an MQTT rule",
        "requestBody": {"required": true, "content": {"application/json": {}}},
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/airplay": {
      "get": {
        "tags": ["config"],
        "summary": "Get AirPlay receiver configuration",
        "responses": {"200": {"description": "AirPlay config with enabled flag, base_port, and model"}}
      },
      "put": {
        "tags": ["config"],
        "summary": "Update AirPlay receiver configuration (requires restart)",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"enabled": {"type": "boolean"}, "base_port": {"type": "integer"}, "prime_silence_ms": {"type": "integer"}, "model": {"type": "string", "description": "Device model advertised over mDNS; controls the iOS AirPlay icon", "example": "RealityDevice14,1"}, "gain": {"type": "number", "default": 1.0, "description": "Digital gain applied to AirPlay audio before sending to camera"}}}}}},
        "responses": {"200": {"description": "Updated — restart required for changes to take effect"}}
      }
    },
    "/mqtt/status": {
      "get": {
        "tags": ["mqtt"],
        "summary": "MQTT connection status",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/mqtt/events": {
      "get": {
        "tags": ["mqtt"],
        "summary": "SSE stream of MQTT messages",
        "responses": {"200": {"description": "SSE stream"}}
      }
    },
    "/mqtt/topics": {
      "get": {
        "tags": ["mqtt"],
        "summary": "All topics seen by the broker since startup",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/mqtt/subscribe": {
      "post": {
        "tags": ["mqtt"],
        "summary": "Dynamically subscribe to a topic at runtime",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"topic": {"type": "string"}}}}}},
        "responses": {"200": {"description": "OK"}}
      }
    }
  },
  "components": {
    "schemas": {
      "CameraInfo": {
        "type": "object",
        "description": "Vendor-neutral camera settings (device info, streaming config, network).",
        "properties": {
          "type": {"type": "string", "example": "hikvision"},
          "online": {"type": "boolean"},
          "device": {"$ref": "#/components/schemas/DeviceInfo"},
          "network": {"$ref": "#/components/schemas/NetworkInfo"},
          "streams": {"type": "array", "items": {"$ref": "#/components/schemas/StreamInfo"}},
          "errors": {"type": "array", "items": {"type": "string"}}
        }
      },
      "DeviceInfo": {
        "type": "object",
        "properties": {
          "manufacturer": {"type": "string", "example": "Hikvision"},
          "model": {"type": "string", "example": "DS-2CD2042WD-I"},
          "firmware": {"type": "string", "example": "V5.5.0"},
          "serial": {"type": "string"},
          "device_type": {"type": "string"},
          "hardware": {"type": "string"}
        }
      },
      "NetworkInfo": {
        "type": "object",
        "properties": {
          "ip": {"type": "string", "example": "192.168.1.100"},
          "mac": {"type": "string", "example": "aa:bb:cc:dd:ee:ff"},
          "gateway": {"type": "string"},
          "subnet": {"type": "string"},
          "dns": {"type": "string"}
        }
      },
      "StreamInfo": {
        "type": "object",
        "properties": {
          "channel": {"type": "integer", "example": 1},
          "name": {"type": "string", "example": "Camera 01"},
          "video": {"$ref": "#/components/schemas/VideoInfo"},
          "audio": {"$ref": "#/components/schemas/AudioInfo"}
        }
      },
      "VideoInfo": {
        "type": "object",
        "properties": {
          "codec": {"type": "string", "example": "H.264"},
          "resolution": {"type": "string", "example": "1920x1080"},
          "width": {"type": "integer", "example": 1920},
          "height": {"type": "integer", "example": 1080},
          "framerate": {"type": "integer", "description": "fps", "example": 25},
          "bitrate": {"type": "integer", "description": "kbps", "example": 4096},
          "bitrate_type": {"type": "string", "example": "VBR"},
          "gop": {"type": "integer", "example": 50},
          "profile": {"type": "string", "example": "main"}
        }
      },
      "AudioInfo": {
        "type": "object",
        "properties": {
          "codec": {"type": "string", "example": "G.711ulaw"},
          "sample_rate": {"type": "integer", "description": "Hz", "example": 8000},
          "bitrate": {"type": "integer", "description": "kbps", "example": 64},
          "channels": {"type": "integer", "example": 1}
        }
      },
      "SpeakRequest": {
        "type": "object",
        "required": ["camera", "text"],
        "properties": {
          "camera": {"type": "string", "description": "Camera name", "example": "backyard"},
          "text": {"type": "string", "description": "Text to speak", "example": "Hello world"},
          "voice": {"type": "string", "description": "TTS voice (empty = default)", "example": "af_sky"},
          "gain": {"type": "number", "description": "Audio gain multiplier", "default": 3.0, "example": 3.0}
        }
      },
      "BroadcastRequest": {
        "type": "object",
        "properties": {
          "text": {"type": "string", "example": "Announcement text"},
          "voice": {"type": "string", "example": "af_sky"},
          "preset": {"type": "string", "description": "Preset name (alternative to text)"},
          "gain": {"type": "number", "default": 3.0}
        }
      },
      "PlayRequest": {
        "type": "object",
        "required": ["camera", "preset"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "preset": {"type": "string", "example": "person_detected"},
          "category": {"type": "string", "example": "alerts"},
          "gain": {"type": "number", "default": 3.0},
          "loop": {"type": "boolean", "default": false, "description": "If true, loop the preset infinitely. Uses ffmpeg -stream_loop -1, so the loop can be paused/resumed/stopped like a live stream via /api/pause, /api/resume, /api/stop."}
        }
      },
      "PlayURLRequest": {
        "type": "object",
        "required": ["camera", "url"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "url": {"type": "string", "example": "http://host/audio.wav"},
          "gain": {"type": "number", "default": 3.0}
        }
      },
      "CameraRequest": {
        "type": "object",
        "required": ["camera"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"}
        }
      },
      "VisionRequest": {
        "type": "object",
        "required": ["camera"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "prompt": {"type": "string", "description": "Vision prompt (empty = camera/global default)", "example": "How many people do you see?"}
        }
      },
      "VisionResponse": {
        "type": "object",
        "properties": {
          "description": {"type": "string", "example": "There are 2 people in the driveway."}
        }
      },
      "VisionTestRequest": {
        "type": "object",
        "properties": {
          "camera": {"type": "string", "description": "Required if image is empty (to capture snapshot)", "example": "backyard"},
          "prompt": {"type": "string", "description": "Vision prompt to test", "example": "Describe what you see in one or two sentences."},
          "image": {"type": "string", "description": "Base64 data URI of a cached image. If provided, skips snapshot capture and reuses this image.", "example": "data:image/jpeg;base64,/9j/4AAQ..."}
        }
      },
      "VisionTestResponse": {
        "type": "object",
        "properties": {
          "description": {"type": "string", "example": "A white minivan parked in a driveway."},
          "image": {"type": "string", "description": "Base64 data URI of the image used (for client-side caching and display)", "example": "data:image/jpeg;base64,/9j/4AAQ..."}
        }
      },
      "DescribeRequest": {
        "type": "object",
        "required": ["camera"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "prompt": {"type": "string", "example": "Describe what you see."},
          "gain": {"type": "number", "default": 3.0}
        }
      },
      "DescribeResponse": {
        "type": "object",
        "properties": {
          "status": {"type": "string", "example": "ok"},
          "description": {"type": "string", "example": "A car is parked in the driveway."},
          "image": {"type": "string", "description": "Base64 JPEG data URI"}
        }
      },
      "StatusResponse": {
        "type": "object",
        "properties": {
          "status": {"type": "string", "example": "ok"}
        }
      },
      "HealthResponse": {
        "type": "object",
        "properties": {
          "status": {"type": "string", "example": "ok"},
          "version": {"type": "string", "example": "v1.10.0"}
        }
      },
      "EventEntry": {
        "type": "object",
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "action": {"type": "string", "example": "speak"},
          "text": {"type": "string", "example": "Hello world"},
          "voice": {"type": "string", "example": "af_sky"},
          "at": {"type": "string", "format": "date-time", "example": "2026-08-13T13:40:43Z"}
        }
      },
      "Camera": {
        "type": "object",
        "properties": {
          "name": {"type": "string", "example": "backyard"},
          "type": {"type": "string", "example": "hikvision"},
          "online": {"type": "boolean", "example": true}
        }
      },
      "CameraConfig": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "type": {"type": "string", "enum": ["hikvision", "reolink", "go2rtc", "onvif"]},
          "ip": {"type": "string"},
          "user": {"type": "string"},
          "pass": {"type": "string"},
          "channel": {"type": "integer", "default": 1},
          "stream": {"type": "string"},
          "enabled": {"type": "boolean", "default": false},
          "vision_prompt": {"type": "string"}
        }
      },
      "Preset": {
        "type": "object",
        "properties": {
          "name": {"type": "string", "example": "person_detected"},
          "category": {"type": "string", "example": "alerts"},
          "duration": {"type": "number", "example": 1.4},
          "text": {"type": "string", "example": "Person detected"}
        }
      },
      "UploadJobAccepted": {
        "type": "object",
        "properties": {
          "job_id": {"type": "string", "example": "20260813-114407-1"},
          "status": {"type": "string", "example": "transcoding"},
          "name": {"type": "string", "example": "my_audio"},
          "category": {"type": "string", "example": "uploads"},
          "filename": {"type": "string", "example": "recording.mp3"}
        }
      },
      "UploadJob": {
        "type": "object",
        "properties": {
          "id": {"type": "string", "example": "20260813-114407-1"},
          "status": {"type": "string", "enum": ["transcoding", "saving", "done", "error"], "example": "transcoding"},
          "percent": {"type": "number", "description": "0–100, or -1 for indeterminate", "example": 45.2},
          "step": {"type": "string", "example": "Transcoding"},
          "name": {"type": "string", "example": "my_audio"},
          "category": {"type": "string", "example": "uploads"},
          "filename": {"type": "string", "example": "recording.mp3"},
          "error": {"type": "string", "description": "Present only when status is error"},
          "preset": {"$ref": "#/components/schemas/Preset"},
          "started_at": {"type": "string", "format": "date-time"},
          "done_at": {"type": "string", "format": "date-time"}
        }
      },
      "GeneratePresetRequest": {
        "type": "object",
        "required": ["name", "text"],
        "properties": {
          "name": {"type": "string", "example": "person_detected"},
          "text": {"type": "string", "example": "Person detected"},
          "category": {"type": "string", "default": "alerts"},
          "voice": {"type": "string", "example": "af_sky"}
        }
      },
      "TTSPreviewRequest": {
        "type": "object",
        "required": ["text"],
        "properties": {
          "text": {"type": "string", "example": "Hello world"},
          "voice": {"type": "string", "example": "af_sky"}
        }
      },
      "VisionConfig": {
        "type": "object",
        "properties": {
          "url": {"type": "string", "example": "http://10.0.0.x:8080/v1/chat/completions"},
          "model": {"type": "string", "example": "llama3.2-vision"},
          "api_key": {"type": "string"},
          "prompt": {"type": "string", "description": "Global default vision prompt"}
        }
      },
      "VisionPromptPreset": {
        "type": "object",
        "required": ["name"],
        "properties": {
          "name": {"type": "string", "example": "concise-people"},
          "prompt": {"type": "string", "example": "Describe what you see in one or two sentences. Focus on people, vehicles, and animals."},
          "description": {"type": "string", "example": "Concise description focusing on people and vehicles"}
        }
      },
      "TTSPreset": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "endpoint": {"type": "string"},
          "model": {"type": "string"},
          "default_voice": {"type": "string"},
          "description": {"type": "string"}
        }
      },
      "PlaybackState": {
        "type": "object",
        "properties": {
          "state": {"type": "string", "enum": ["playing", "paused", "idle"], "example": "playing"},
          "source": {"type": "string", "enum": ["stream", "speak", "play", "play-url", "beep"], "example": "stream"},
          "detail": {"type": "string", "example": "http://liveatc.net/stream.m3u"},
          "started_at": {"type": "string", "format": "date-time"},
          "paused_at": {"type": "string", "format": "date-time", "description": "Present only when state is paused"}
        }
      }
    }
  }
}`
