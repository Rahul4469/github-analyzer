#!/bin/bash
# This script creates all AWS resources needed for deployment:
# - ECR Repository (Docker image storage)
# - Security Group (firewall rules)
# - Key Pair (SSH access)
# - IAM Role (EC2 permissions)
# - EC2 Instance (server)
# - Elastic IP (static IP address)

set -e # Exit on any error

# Configurations | base vairables ---------------------------------

AWS_REGION="${AWS_REGION:-ap-south-1}"
PROJECT_NAME="github-analyzer"
EC2_INSTANCE_TYPE="t3.micro"
KEY_NAME="${PROJECT_NAME}-key"

# Print configs
echo "---GitHub Analyzer - AWS Infrastructure Setup---"
echo ""
echo "Configuration:"
echo "  Region:        $AWS_REGION"
echo "  Project:       $PROJECT_NAME"
echo "  Instance Type: $EC2_INSTANCE_TYPE"
echo ""

# 1. CREATE ECR REPOSITORY ----------------------------------------
# ECR is private by default, integrates with IAM for access/auth
echo "[1/6] Creating ECR Repository..."
echo "────────────────────────────────────────────────────────────"

# Check if repository already exists ????????????????
ECR_EXISTS=$(aws ecr describe-repositories \
    --repository-names $PROJECT_NAME \
    --region $AWS_REGION 2>/dev/null || echo "") # redirect/discard err message to dev/null

if [ -z "$ECR_EXISTS" ]; then 
    # Create new ECR repository
    # --image-scanning-configuration: Automatically scan images for vulnerabilities
    # --encryption-configuration: Encrypt images at rest
    aws ecr create-repository \
        --repository-name $PROJECT_NAME \
        --region $AWS_REGION \
        --image-scanning-configuration scanOnPush=true \
        --encryption-configuration encryptionType=AES256 \
        --output text
        echo "  ECR repository created: $PROJECT_NAME"
else
    echo "  ECR repository already exists: $PROJECT_NAME"
fi

# Get ECR URI for later use
ECR_URI=$(aws ecr describe-repositories \
    --repository-names $PROJECT_NAME \
    --region $AWS_REGION \
    --query 'repositories[0].repositoryUri' \
    --output text)

echo "  ECR URI: $ECR_URI"
echo ""

# 2. GET DEFAULT VPC AND CREATE SECURITY GROUPS -------------------

# SSH: 22, HTTP: 80, HTTPS: 443
echo "[2/6] Creating Security Group..."
echo "────────────────────────────────────────────────────────────"

VPC_ID=$(aws ec2 describe-vpcs \
    --filters "Name=isDefault,Values=true" \
    --query 'Vpcs[0].VpcId' \
    --output text \
    --region $AWS_REGION)

 echo "  VPC ID: $VPC_ID"   

 # Check if security group exists ???????????????????
SG_ID=$(aws ec2 describe-security-groups \
    --filters "Name=group-name,Values=${PROJECT_NAME}-sg" "Name=vpc-id,Values=$VPC_ID" \
    --query 'SecurityGroups[0].GroupId' \
    --output text \
    --region $AWS_REGION 2>/dev/null || echo "None")

if [ "$SG_ID" == "None" ] || [ -z "$SG_ID" ]; then
    # CREATE security group
    SG_ID=$(aws ec2 create-security-group \
        --group-name "${PROJECT_NAME}-sg" \
        --description "Security group for GitHub Analyzer" \
        --vpc-id $VPC_ID \
        --query 'GroupId' \
        --output text \
        --region $AWS_REGION)    

    # INBOUND RULES
    # ─────────────
    # Allow SSH (port 22) from anywhere
    # In production, restrict to your IP: --cidr YOUR_IP/32
    aws ec2 authorize-security-group-ingress \
        --group-id $SG_ID \
        --protocol tcp \
        --port 22 \
        --cidr 0.0.0.0/0 \
        --region $AWS_REGION
    
    # Allow HTTP (port 80) from anywhere
    # Needed for: Let's Encrypt challenge, HTTP->HTTPS redirect
    aws ec2 authorize-security-group-ingress \
        --group-id $SG_ID \
        --protocol tcp \
        --port 80 \
        --cidr 0.0.0.0/0 \
        --region $AWS_REGION
    
    # Allow HTTPS (port 443) from anywhere
    # Main web traffic
    aws ec2 authorize-security-group-ingress \
        --group-id $SG_ID \
        --protocol tcp \
        --port 443 \
        --cidr 0.0.0.0/0 \
        --region $AWS_REGION    

    # NOTE: Outbound is allowed by default (all traffic)
    # Database port (5432) is NOT exposed - only internal Docker access

    echo "      Security group created: $SG_ID"
    echo "    - Port 22 (SSH): Open"
    echo "    - Port 80 (HTTP): Open"
    echo "    - Port 443 (HTTPS): Open"
else
    echo "    Security group already exists: $SG_ID"
fi
echo ""

# 3. CREATE KEY PAIR | SSH ----------------------------------------
echo "[3/6] Creating Key Pair..."
echo "────────────────────────────────────────────────────────────"

# Create .ssh directory if it doesn't exist
mkdir -p ~/.ssh

# Check if key pair exists in AWS ??????????????????
KEY_EXISTS=$(aws ec2 describe-key-pairs \
    --key-names $KEY_NAME \
    --region $AWS_REGION 2>/dev/null || echo "")

if [ -z "$KEY_EXISTS" ]; then
    # Create key pair and save private key
    aws ec2 create-key-pair \
        --key-name $KEY_NAME \
        --query 'KeyMaterial' \
        --output text \
        --region $AWS_REGION > ~/.ssh/${KEY_NAME}.pem
    
    # Set restrictive permissions (required by SSH)
    # 400 = owner read-only
    chmod 400 ~/.ssh/${KEY_NAME}.pem

    echo "  Key pair created"
    echo "  Private key saved: ~/.ssh/${KEY_NAME}.pem"
    echo ""
    echo "  IMPORTANT: Back up this key! It cannot be recovered!"
else
    echo "  Key pair already exists: $KEY_NAME"
    if [ -f ~/.ssh/${KEY_NAME}.pem ]; then
        echo "  Private key: ~/.ssh/${KEY_NAME}.pem"
    else
        echo "  Private key not found locally!"
    fi
fi
echo ""    

# 4. CREATE IAM ROLE FOR EC2 ----------------------------------
# IAM Role to allow EC2 to access AWS services without storing credentials
# Gets temporary credentials automatically
# We need ECR read access to pull Docker images
echo "[4/6] Creating IAM Role..."
echo "────────────────────────────────────────────────────────────"

ROLE_NAME="${PROJECT_NAME}-ec2-role"

# Check if role exists ??????????????????????????
ROLE_EXISTS=$(aws iam get-role --role-name $ROLE_NAME 2>/dev/null || echo "")

if [ -z "$ROLE_EXISTS" ]; then
    # Create trust policy (who can assume this role)
    # This policy says: "EC2 service can assume this role"
    TRUST_POLICY=$(cat << 'EOF'
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Service": "ec2.amazonaws.com"
            },
            "Action": "sts:AssumeRole"
        }
    ]
}
EOF
)

    # Create the role
    aws iam create-role \
        --role-name $ROLE_NAME \
        --assume-role-policy-document "$TRUST_POLICY" \
        --output text

    # Attach policy: Allow reading from ECR
    # This is an AWS-managed policy for ECR read access
    aws iam attach-role-policy \
        --role-name $ROLE_NAME \
        --policy-arn arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly

    # Create instance profile (required to attach role to EC2)
    # Instance profile = container for IAM role that EC2 can use
    aws iam create-instance-profile \
        --instance-profile-name $ROLE_NAME

    # Add role to instance profile
    aws iam add-role-to-instance-profile \
        --instance-profile-name $ROLE_NAME \
        --role-name $ROLE_NAME

    echo "  IAM role created: $ROLE_NAME"
    echo "  Waiting for IAM to propagate..."
    sleep 15  # IAM changes take time to propagate
else
    echo "  IAM role already exists: $ROLE_NAME"
fi
echo ""

# 5. LAUNCH EC2 INSTANCE -------------------------------------------
# t3.micro instance with Amazon linux 2023

echo "[5/6] Launching EC2 Instance..."
echo "────────────────────────────────────────────────────────────"
# Get latest Amazon Linux 2023 AMI
# AMI = Amazon Machine Image
AMI_ID=$(aws ec2 describe-images \
    --owners amazon \
    --filters "Name=name,Values=al2023-ami-*-x86_64" "Name=state,Values=available" \
    --query 'sort_by(Images, &CreationDate)[-1].ImageId' \
    --output text \
    --region $AWS_REGION)

echo "  AMI ID: $AMI_ID (Amazon Linux 2023)"

# Check if instance already exists
INSTANCE_ID=$(aws ec2 describe-instances \
    --filters "Name=tag:Name,Values=${PROJECT_NAME}" "Name=instance-state-name,Values=running,pending,stopping,stopped" \
    --query 'Reservations[0].Instances[0].InstanceId' \
    --output text \
    --region $AWS_REGION 2>/dev/null || echo "None")

if [ "$INSTANCE_ID" == "None" ] || [ -z "$INSTANCE_ID" ]; then
    # User data script: Runs on first boot
    # Installs Docker, Docker Compose, and basic setup
    USER_DATA=$(cat << 'USERDATA'
#!/bin/bash
set -e

# Log all output
exec > >(tee /var/log/user-data.log) 2>&1

echo "Starting EC2 initialization..."

# Update system packages
dnf update -y

# Install Docker
dnf install -y docker

# Start Docker and enable on boot
systemctl start docker
systemctl enable docker

# Add ec2-user to docker group (run docker without sudo)
usermod -aG docker ec2-user

# Install Docker Compose
COMPOSE_VERSION=$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep -Po '"tag_name": "\K.*?(?=")')
curl -L "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose
ln -sf /usr/local/bin/docker-compose /usr/bin/docker-compose

# Install AWS CLI (for ECR login)
dnf install -y aws-cli

# Create app directory
mkdir -p /home/ec2-user/github-analyzer
chown -R ec2-user:ec2-user /home/ec2-user/github-analyzer

# Install useful tools
dnf install -y htop vim wget

echo "EC2 initialization complete!"
USERDATA
)

    # Launch the instance
    INSTANCE_ID=$(aws ec2 run-instances \
        --image-id $AMI_ID \
        --instance-type $EC2_INSTANCE_TYPE \
        --key-name $KEY_NAME \
        --security-group-ids $SG_ID \
        --iam-instance-profile Name=$ROLE_NAME \
        --user-data "$USER_DATA" \
        --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=${PROJECT_NAME}}]" \
        --block-device-mappings "[{\"DeviceName\":\"/dev/xvda\",\"Ebs\":{\"VolumeSize\":30,\"VolumeType\":\"gp3\"}}]" \
        --query 'Instances[0].InstanceId' \
        --output text \
        --region $AWS_REGION)

    echo "  Instance launched: $INSTANCE_ID"
    echo "  Waiting for instance to be running..."    
    
    # Wait for instance to be in running state
    aws ec2 wait instance-running \
        --instance-ids $INSTANCE_ID \
        --region $AWS_REGION
    echo "  Instance is running!"
else
    echo "  Instance already exists: $INSTANCE_ID"
fi   

# Get current public IP
PUBLIC_IP=$(aws ec2 describe-instances \
    --instance-ids $INSTANCE_ID \
    --query 'Reservations[0].Instances[0].PublicIpAddress' \
    --output text \
    --region $AWS_REGION)

echo "  Public IP: $PUBLIC_IP"
echo ""

# 6. ALLOCATE ELASTIC IP ++++++++++++++++++++++++++++++++++++++++++
# Elastic IP = Static IP address
# Without it, IP changes every time instance stops/starts
# Required for DNS (point domain to fixed IP)
echo "[6/6] Allocating Elastic IP..."
echo "────────────────────────────────────────────────────────────"

# Check if instance already has an Elastic IP?????????????????
EIP_ALLOC=$(aws ec2 describe-addresses \
    --filters "Name=instance-id,Values=$INSTANCE_ID" \
    --query 'Addresses[0].AllocationId' \
    --output text \
    --region $AWS_REGION 2>/dev/null || echo "None")

if [ "$EIP_ALLOC" == "None" ] || [ -z "$EIP_ALLOC" ]; then
    # Allocate new Elastic IP
    EIP_ALLOC=$(aws ec2 allocate-address \
        --domain vpc \
        --query 'AllocationId' \
        --output text \
        --region $AWS_REGION)
    
    # Associate with our instance
    aws ec2 associate-address \
        --instance-id $INSTANCE_ID \
        --allocation-id $EIP_ALLOC \
        --region $AWS_REGION
    
    echo "  Elastic IP allocated and associated"
else
    echo "  Elastic IP already associated"
fi    

# Get the Elastic IP address
ELASTIC_IP=$(aws ec2 describe-addresses \
    --allocation-ids $EIP_ALLOC \
    --query 'Addresses[0].PublicIp' \
    --output text \
    --region $AWS_REGION)

echo "  Elastic IP: $ELASTIC_IP"
echo ""

# GET AWS ACCOUNT ID ----------------------------------
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

# SUMMARY
# ==============================================================================
echo ""
echo "============================================================"
echo " {~} AWS INFRASTRUCTURE SETUP COMPLETE!" {~}
echo "============================================================"
echo ""
echo "RESOURCES CREATED:"
echo "──────────────────"
echo "  ECR Repository:  $ECR_URI"
echo "  Security Group:  $SG_ID"
echo "  Key Pair:        $KEY_NAME"
echo "  IAM Role:        $ROLE_NAME"
echo "  EC2 Instance:    $INSTANCE_ID"
echo "  Elastic IP:      $ELASTIC_IP"
echo ""
echo "AWS ACCOUNT INFO:"
echo "──────────────────"
echo "  Account ID:      $AWS_ACCOUNT_ID"
echo "  Region:          $AWS_REGION"
echo ""
echo "============================================================"
echo "  NEXT STEPS"
echo "============================================================"
echo ""
echo "1. WAIT for EC2 initialization (2-3 minutes)"
echo "   Docker is being installed automatically."
echo ""
echo "2. SSH into the instance:"
echo "   ────────────────────────────────────────────────────────"
echo "   ssh -i ~/.ssh/${KEY_NAME}.pem ec2-user@$ELASTIC_IP"
echo "   ────────────────────────────────────────────────────────"
echo ""
echo "3. Add these SECRETS to your GitHub repository:"
echo "   (Settings → Secrets → Actions → New repository secret)"
echo "   ────────────────────────────────────────────────────────"
echo "   AWS_ACCESS_KEY_ID:      Your AWS access key"
echo "   AWS_SECRET_ACCESS_KEY:  Your AWS secret key"
echo "   EC2_HOST:               $ELASTIC_IP"
echo "   EC2_USER:               ec2-user"
echo "   EC2_SSH_KEY:            (paste contents of ~/.ssh/${KEY_NAME}.pem)"
echo "   ────────────────────────────────────────────────────────"
echo ""
echo "4. Run the EC2 setup script on the server:"
echo "   See: scripts/aws/setup-ec2.sh"
echo ""
echo "============================================================"

# Save configuration for other scripts
# Use TMPDIR if set, otherwise fall back to /tmp
CONFIG_FILE="${TMPDIR:-/tmp}/aws-config.env"
cat > "$CONFIG_FILE" << EOF
AWS_REGION=$AWS_REGION
AWS_ACCOUNT_ID=$AWS_ACCOUNT_ID
ECR_URI=$ECR_URI
INSTANCE_ID=$INSTANCE_ID
ELASTIC_IP=$ELASTIC_IP
KEY_NAME=$KEY_NAME
SG_ID=$SG_ID
EOF

echo ""
echo "Configuration saved to: $CONFIG_FILE"
echo ""