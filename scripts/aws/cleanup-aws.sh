#!/bin/bash
This script removes ALL AWS resources created by setup-aws.sh

set -e

AWS_REGION="${AWS_REGION:-ap-south-1}"
PROJECT_NAME="github-analyzer"

echo "============================================================"
echo "  GitHub Analyzer - AWS Cleanup"
echo "============================================================"
echo ""
echo "   WARNING: This will permanently delete:"
echo "    - EC2 instance and all data"
echo "    - Elastic IP"
echo "    - Security group"
echo "    - Key pair"
echo "    - IAM role"
echo "    - ECR repository and all images"
echo ""
echo "Press Ctrl+C to cancel, or wait 10 seconds to continue..."
echo ""

for i in {10..1}; do
    echo -ne "\r  Continuing in $i seconds... "
    sleep 1
done
echo ""
echo ""

# ==============================================================================
# Step 1: Terminate EC2 Instance
echo "[1/6] Terminating EC2 Instance..."
echo "────────────────────────────────────────────────────────────"

INSTANCE_ID=$(aws ec2 describe-instances \
    --filters "Name=tag:Name,Values=${PROJECT_NAME}" \
              "Name=instance-state-name,Values=running,pending,stopping,stopped" \
    --query 'Reservations[0].Instances[0].InstanceId' \
    --output text \
    --region $AWS_REGION 2>/dev/null || echo "None")

if [ "$INSTANCE_ID" != "None" ] && [ -n "$INSTANCE_ID" ] && [ "$INSTANCE_ID" != "null" ]; then
    # First, release any associated Elastic IP
    EIP_ALLOC=$(aws ec2 describe-addresses \
        --filters "Name=instance-id,Values=$INSTANCE_ID" \
        --query 'Addresses[0].AllocationId' \
        --output text \
        --region $AWS_REGION 2>/dev/null || echo "None")
    
    if [ "$EIP_ALLOC" != "None" ] && [ -n "$EIP_ALLOC" ] && [ "$EIP_ALLOC" != "null" ]; then
        aws ec2 release-address --allocation-id $EIP_ALLOC --region $AWS_REGION
        echo "    Released Elastic IP"
    fi
    
    # Terminate the instance
    aws ec2 terminate-instances --instance-ids $INSTANCE_ID --region $AWS_REGION
    echo "    Terminating instance: $INSTANCE_ID"
    
    echo "  Waiting for termination..."
    aws ec2 wait instance-terminated --instance-ids $INSTANCE_ID --region $AWS_REGION
    echo "    Instance terminated"
else
    echo "  No instance found"
fi
echo ""

# ==============================================================================
# Step 2: Delete Security Group
echo "[2/6] Deleting Security Group..."
echo "────────────────────────────────────────────────────────────"

VPC_ID=$(aws ec2 describe-vpcs \
    --filters "Name=isDefault,Values=true" \
    --query 'Vpcs[0].VpcId' \
    --output text \
    --region $AWS_REGION)

SG_ID=$(aws ec2 describe-security-groups \
    --filters "Name=group-name,Values=${PROJECT_NAME}-sg" "Name=vpc-id,Values=$VPC_ID" \
    --query 'SecurityGroups[0].GroupId' \
    --output text \
    --region $AWS_REGION 2>/dev/null || echo "None")

if [ "$SG_ID" != "None" ] && [ -n "$SG_ID" ] && [ "$SG_ID" != "null" ]; then
    # Wait a bit for instance to fully terminate
    sleep 5
    aws ec2 delete-security-group --group-id $SG_ID --region $AWS_REGION
    echo "    Deleted security group: $SG_ID"
else
    echo "  No security group found"
fi
echo ""

# ==============================================================================
# Step 3: Delete Key Pair
echo "[3/6] Deleting Key Pair..."
echo "────────────────────────────────────────────────────────────"

KEY_NAME="${PROJECT_NAME}-key"
aws ec2 delete-key-pair --key-name $KEY_NAME --region $AWS_REGION 2>/dev/null || true
rm -f ~/.ssh/${KEY_NAME}.pem 2>/dev/null || true
echo "    Deleted key pair: $KEY_NAME"
echo ""

# ==============================================================================
# Step 4: Delete IAM Role
echo "[4/6] Deleting IAM Role..."
echo "────────────────────────────────────────────────────────────"

ROLE_NAME="${PROJECT_NAME}-ec2-role"

# Remove role from instance profile
aws iam remove-role-from-instance-profile \
    --instance-profile-name $ROLE_NAME \
    --role-name $ROLE_NAME 2>/dev/null || true

# Delete instance profile
aws iam delete-instance-profile \
    --instance-profile-name $ROLE_NAME 2>/dev/null || true

# Detach policies
aws iam detach-role-policy \
    --role-name $ROLE_NAME \
    --policy-arn arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly 2>/dev/null || true

# Delete role
aws iam delete-role --role-name $ROLE_NAME 2>/dev/null || true
echo "    Deleted IAM role: $ROLE_NAME"
echo ""

# ==============================================================================
# Step 5: Delete ECR Repository
echo "[5/6] Deleting ECR Repository..."
echo "────────────────────────────────────────────────────────────"

aws ecr delete-repository \
    --repository-name $PROJECT_NAME \
    --force \
    --region $AWS_REGION 2>/dev/null || true
echo "    Deleted ECR repository: $PROJECT_NAME"
echo ""

# ==============================================================================
# Step 6: Delete IAM User (GitHub Actions)
echo "[6/6] Deleting GitHub Actions IAM User..."
echo "────────────────────────────────────────────────────────────"

# List and delete access keys
ACCESS_KEYS=$(aws iam list-access-keys \
    --user-name github-actions \
    --query 'AccessKeyMetadata[*].AccessKeyId' \
    --output text 2>/dev/null || echo "")

for KEY_ID in $ACCESS_KEYS; do
    aws iam delete-access-key \
        --user-name github-actions \
        --access-key-id $KEY_ID 2>/dev/null || true
done

# Detach policies
aws iam detach-user-policy \
    --user-name github-actions \
    --policy-arn arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser 2>/dev/null || true

aws iam detach-user-policy \
    --user-name github-actions \
    --policy-arn arn:aws:iam::aws:policy/AmazonEC2ReadOnlyAccess 2>/dev/null || true

# Delete user
aws iam delete-user --user-name github-actions 2>/dev/null || true
echo "    Deleted IAM user: github-actions"
echo ""

# ==============================================================================
# Summary
echo "============================================================"
echo "    CLEANUP COMPLETE"
echo "============================================================"
echo ""
echo "All AWS resources have been deleted."
echo ""
echo "Remember to also:"
echo "  - Remove GitHub secrets from your repository"
echo "  - Delete any local .env files with secrets"
echo ""
