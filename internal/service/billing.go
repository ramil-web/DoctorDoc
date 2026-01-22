package service

import (
    "context"
)

func (s *fileService) CheckOnlySubscription(ctx context.Context, ip string, fp string) (bool, error) {
    machineID := s.GenerateHardwareHash(ip)
    active, err := s.repo.CheckActiveSubscription(ip, machineID)
    if !active {
        active, _ = s.repo.CheckActiveSubscription(ip, fp)
    }
    return active, err
}

func (s *fileService) CheckOnlyLimit(ctx context.Context, fingerprint string, ip string) (bool, error) {
    return s.CanUpload(ctx, fingerprint, ip, 0)
}