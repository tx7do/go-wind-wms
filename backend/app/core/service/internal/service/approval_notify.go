package service

import (
	"context"
	"fmt"

	"go-wind-wms/app/core/service/internal/data"

	approvalV1 "go-wind-wms/api/gen/go/approval/service/v1"
	internalMessageV1 "go-wind-wms/api/gen/go/internal_message/service/v1"
)

// approvalNotifier 审批事件站内信通知（审结时通知申请人）。
// 通知失败仅记录——通知是尽力而为的辅助通道，不阻塞审批主流程。
type approvalNotifier struct {
	messageRepo   *data.InternalMessageRepo
	recipientRepo *data.InternalMessageRecipientRepo
	log           interface {
		Errorf(format string, args ...any)
	}
}

// notifyResolved 审结通知：标题带结果与业务类型，收件人=申请人。
func (n *approvalNotifier) notifyResolved(ctx context.Context, approval *approvalV1.ApprovalRequest, approved bool) error {
	if n == nil || approval.GetApplicantId() == 0 {
		return nil
	}

	result := "已驳回"
	if approved {
		result = "已通过"
	}

	msg, err := n.messageRepo.Create(ctx, &internalMessageV1.CreateInternalMessageRequest{
		Data: &internalMessageV1.InternalMessage{
			Title:   strPtr(fmt.Sprintf("审批%s：%s", result, approval.GetTitle())),
			Content: strPtr(fmt.Sprintf("你的审批单（%s / %s）%s。", approval.GetBizType(), approval.GetBizRef(), result)),
			Status:  internalMessageV1.InternalMessage_PUBLISHED.Enum(),
			Type:    internalMessageV1.InternalMessage_NOTIFICATION.Enum(),
		},
	})
	if err != nil {
		return err
	}

	_, err = n.recipientRepo.Create(ctx, &internalMessageV1.InternalMessageRecipient{
		MessageId:       msg.Id,
		RecipientUserId: approval.ApplicantId,
	})
	return err
}

// notifyStepApprovers 多级审批级进通知：告知下一级候选审批人（持有
// role_code 的用户）有待办。收件人剔除申请人（自审已被拦截）。
func (n *approvalNotifier) notifyStepApprovers(
	ctx context.Context,
	approval *approvalV1.ApprovalRequest,
	step, total uint32,
	userIDs []uint32,
) error {
	if n == nil || len(userIDs) == 0 {
		return nil
	}

	msg, err := n.messageRepo.Create(ctx, &internalMessageV1.CreateInternalMessageRequest{
		Data: &internalMessageV1.InternalMessage{
			Title:   strPtr(fmt.Sprintf("待审批（第 %d/%d 级）：%s", step, total, approval.GetTitle())),
			Content: strPtr(fmt.Sprintf("多级审批推进到你所在级别（%s / %s），请到审批中心处理。", approval.GetBizType(), approval.GetBizRef())),
			Status:  internalMessageV1.InternalMessage_PUBLISHED.Enum(),
			Type:    internalMessageV1.InternalMessage_NOTIFICATION.Enum(),
		},
	})
	if err != nil {
		return err
	}

	for _, uid := range userIDs {
		if uid == approval.GetApplicantId() {
			continue
		}
		if _, err := n.recipientRepo.Create(ctx, &internalMessageV1.InternalMessageRecipient{
			MessageId:       msg.Id,
			RecipientUserId: &uid,
		}); err != nil {
			return err
		}
	}
	return nil
}


// strPtr 字符串转指针（消息构造用）。
func strPtr(s string) *string { return &s }
