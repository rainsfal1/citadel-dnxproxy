#!/bin/bash

# Citadel Demo Runner Script
# This script helps demonstrate Citadel's parental control features

set -e

echo "================================================"
echo "  Citadel/Quietbox - Parental DNS Control Demo"
echo "================================================"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if binary exists
if [ ! -f "./dnsproxy" ]; then
    echo -e "${RED}Error: dnsproxy binary not found${NC}"
    echo "Please run: go build -o dnsproxy ."
    exit 1
fi

# Check if demo config exists
if [ ! -f "./demo_config.json" ]; then
    echo -e "${RED}Error: demo_config.json not found${NC}"
    exit 1
fi

echo -e "${BLUE}Step 1: Validating Configuration${NC}"
echo "----------------------------------------"
./dnsproxy -t demo_config.json
echo ""

echo -e "${GREEN}✓ Configuration is valid!${NC}"
echo ""

echo -e "${BLUE}Step 2: Demo Devices Configured${NC}"
echo "----------------------------------------"
echo "1. Ali's iPad      (192.168.1.100) - Internet 4pm-8pm only"
echo "   - Facebook: Always blocked"
echo "   - YouTube: Only 6-7pm"
echo "   - TikTok: Always blocked"
echo ""
echo "2. Zara's Phone    (192.168.1.101) - Internet 3pm-9pm only"
echo "   - TikTok: Always blocked"
echo ""
echo "3. Ahmed's Laptop  (192.168.1.102) - No restrictions (parent)"
echo ""
echo "4. Guest Tablet    (192.168.1.103) - Completely blocked"
echo ""

echo -e "${BLUE}Step 3: Starting DNS Proxy${NC}"
echo "----------------------------------------"
echo -e "${YELLOW}Starting on port 5353 (non-privileged)${NC}"
echo "To use standard DNS port 53, run: sudo ./dnsproxy demo_config.json"
echo ""
echo "Press Ctrl+C to stop the server"
echo ""
echo "In another terminal, test with:"
echo "  dig @localhost -p 5353 facebook.com"
echo "  dig @localhost -p 5353 google.com"
echo ""
echo "View logs with:"
echo "  tail -f citadel_demo.log"
echo ""
echo -e "${GREEN}Starting server...${NC}"
echo "========================================"
echo ""

./dnsproxy demo_config.json
