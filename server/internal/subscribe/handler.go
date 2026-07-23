package subscribe

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mochi-ai/server/internal/catalog"
	"github.com/mochi-ai/server/internal/middleware"
)

type Handler struct {
	catalog   *catalog.Service
	subscribe *Service
}

func NewHandler(catalogSvc *catalog.Service, subscribeSvc *Service) *Handler {
	return &Handler{catalog: catalogSvc, subscribe: subscribeSvc}
}

func (h *Handler) ListSKUs(c *gin.Context) {
	skus, err := h.catalog.ListEnabled(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"skus": skus})
}

func (h *Handler) Adopt(c *gin.Context) {
	userID := middleware.UserID(c)
	var req struct {
		SKUId   string `json:"sku_id" binding:"required"`
		PetName string `json:"pet_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.subscribe.Adopt(c.Request.Context(), userID, AdoptInput{
		SKUId:   req.SKUId,
		PetName: req.PetName,
	})
	if err != nil {
		if err == ErrSKUNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "sku not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"order": result.Order,
		"pet":   result.Pet,
		"sku":   result.SKU,
		"message": "认购成功（支付已跳过）",
	})
}
