package grpc

import (
	"context"
	"errors"

	pb "micro-proto"
	"product-service/internal/service"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Handler struct {
	pb.UnimplementedProductServiceServer
	productService *service.ProductService
}

func NewHandler(productService *service.ProductService) *Handler {
	return &Handler{productService: productService}
}

func (h *Handler) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.GetProductResponse, error) {
	product, err := h.productService.GetProduct(ctx, req.ProductId)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			return nil, status.Errorf(codes.NotFound, "product id %d not found", req.ProductId)
		}
		return nil, status.Errorf(codes.Internal, "get product failed")
	}
	return product, nil
}

func (h *Handler) GetStockDeductionLog(ctx context.Context, req *pb.GetStockDeductionLogRequest) (*pb.GetStockDeductionLogResponse, error) {
	logEntry, err := h.productService.GetStockDeductionLog(ctx, req.OrderId)
	if err != nil {
		if errors.Is(err, service.ErrStockDeductionLogNotFound) {
			return &pb.GetStockDeductionLogResponse{Code: 404, Message: "stock deduction log not found"}, nil
		}
		return &pb.GetStockDeductionLogResponse{Code: 500, Message: "query stock deduction log failed"}, nil
	}

	return &pb.GetStockDeductionLogResponse{
		Code:      200,
		Message:   "ok",
		OrderId:   logEntry.OrderID,
		ProductId: logEntry.ProductID,
		Quantity:  logEntry.Quantity,
		Status:    logEntry.Status,
		Reason:    logEntry.Reason,
	}, nil
}
