package com.artisanlab.asset;

import com.artisanlab.common.ApiException;
import com.artisanlab.config.ArtisanProperties;
import com.artisanlab.canvas.CanvasMapper;
import io.minio.BucketExistsArgs;
import io.minio.GetObjectArgs;
import io.minio.GetObjectResponse;
import io.minio.MakeBucketArgs;
import io.minio.MinioClient;
import io.minio.PutObjectArgs;
import io.minio.RemoveObjectArgs;
import jakarta.annotation.PostConstruct;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.multipart.MultipartFile;

import javax.imageio.ImageIO;
import javax.imageio.ImageWriteParam;
import javax.imageio.ImageWriter;
import javax.imageio.stream.ImageOutputStream;
import java.awt.Color;
import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.image.BufferedImage;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.net.URISyntaxException;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;
import java.util.Locale;
import java.util.UUID;

@Service
public class AssetService {
    private static final Logger log = LoggerFactory.getLogger(AssetService.class);

    private static final int PREVIEW_MAX_SIDE = 1600;
    private static final int THUMBNAIL_MAX_SIDE = 512;
    private static final int MODEL_INPUT_MAX_SIDE = 2048;
    private static final float PREVIEW_WEBP_QUALITY = 0.82f;
    private static final float THUMBNAIL_WEBP_QUALITY = 0.76f;
    private static final float CANVAS_VIEW_WEBP_QUALITY = 0.82f;
    private static final float MODEL_INPUT_JPEG_QUALITY = 0.86f;
    private static final String WEBP_FORMAT = "webp";
    private static final String WEBP_CONTENT_TYPE = "image/webp";
    private static final Duration IMAGE_IMPORT_TIMEOUT = Duration.ofSeconds(60);
    private static final int IMAGE_IMPORT_MAX_BYTES = 32 * 1024 * 1024;

    private final AssetMapper assetMapper;
    private final CanvasMapper canvasMapper;
    private final ArtisanProperties properties;
    private final MinioClient minioClient;
    private final HttpClient httpClient;

    public AssetService(AssetMapper assetMapper, CanvasMapper canvasMapper, ArtisanProperties properties) {
        this.assetMapper = assetMapper;
        this.canvasMapper = canvasMapper;
        this.properties = properties;
        this.minioClient = createMinioClient(properties.minio().endpoint(), properties.minio().accessKey(), properties.minio().secretKey());
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(15))
                .followRedirects(HttpClient.Redirect.NORMAL)
                .build();
    }

    @PostConstruct
    public void ensureBucketExists() {
        try {
            ImageIO.scanForPlugins();
            boolean exists = minioClient.bucketExists(BucketExistsArgs.builder()
                    .bucket(properties.minio().bucket())
                    .build());
            if (!exists) {
                minioClient.makeBucket(MakeBucketArgs.builder()
                        .bucket(properties.minio().bucket())
                        .build());
            }
        } catch (Exception exception) {
            throw new ApiException(HttpStatus.INTERNAL_SERVER_ERROR, "MINIO_UNAVAILABLE", "MinIO 初始化失败");
        }
    }

    @Transactional
    public AssetDtos.AssetResponse upload(UUID userId, UUID canvasId, MultipartFile file) {
        validateImage(file);
        byte[] fileBytes;
        try {
            fileBytes = file.getBytes();
        } catch (IOException exception) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "ASSET_READ_FAILED", "读取上传图片失败");
        }

        return uploadBytes(userId, canvasId, file.getOriginalFilename(), file.getContentType(), fileBytes);
    }

    @Transactional
    public AssetDtos.AssetResponse uploadBytes(
            UUID userId,
            UUID canvasId,
            String originalFilename,
            String contentType,
            byte[] fileBytes
    ) {
        if (fileBytes == null || fileBytes.length == 0) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "EMPTY_FILE", "请上传图片文件");
        }
        if (canvasId != null && canvasMapper.findByIdAndUserIdWithJson(canvasId, userId) == null) {
            throw new ApiException(HttpStatus.NOT_FOUND, "CANVAS_NOT_FOUND", "项目画布不存在");
        }

        UUID assetId = UUID.randomUUID();
        String mimeType = normalizeMimeType(contentType);
        String filename = normalizeFilename(originalFilename);
        BufferedImage image = readImage(fileBytes);
        if (!mimeType.startsWith("image/")) {
            if (image == null) {
                throw new ApiException(HttpStatus.BAD_REQUEST, "UNSUPPORTED_FILE_TYPE", "只支持图片文件");
            }
            mimeType = "image/png";
        }
        String objectKey = "canvases/%s/%s%s".formatted(
                canvasId == null ? "default" : canvasId,
                assetId,
                extensionFor(filename, mimeType)
        );

        ImageSize imageSize = image == null
                ? new ImageSize(null, null)
                : new ImageSize(image.getWidth(), image.getHeight());

        try (InputStream inputStream = new ByteArrayInputStream(fileBytes)) {
            minioClient.putObject(PutObjectArgs.builder()
                    .bucket(properties.minio().bucket())
                    .object(objectKey)
                    .stream(inputStream, fileBytes.length, -1)
                    .contentType(mimeType)
                    .build());
            uploadImageVariants(objectKey, image);
        } catch (Exception exception) {
            throw new ApiException(HttpStatus.BAD_GATEWAY, "ASSET_UPLOAD_FAILED", "图片上传到 MinIO 失败");
        }

        AssetEntity entity = new AssetEntity();
        entity.setId(assetId);
        entity.setCanvasId(canvasId);
        entity.setUserId(userId);
        entity.setObjectKey(objectKey);
        entity.setUrlPath("/api/assets/%s/content".formatted(assetId));
        entity.setOriginalFilename(filename);
        entity.setMimeType(mimeType);
        entity.setSizeBytes(fileBytes.length);
        entity.setWidth(imageSize.width());
        entity.setHeight(imageSize.height());
        assetMapper.insertAsset(entity);

        return toResponse(assetMapper.findById(assetId));
    }

    @Transactional
    public AssetDtos.AssetResponse importFromUrl(UUID userId, UUID canvasId, String rawUrl) {
        String url = rawUrl == null ? "" : rawUrl.trim();
        if (url.isBlank()) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "ASSET_URL_REQUIRED", "图片地址不能为空");
        }

        UUID existingAssetId = parseAssetIdFromUrl(url);
        if (existingAssetId != null) {
            AssetEntity existing = assetMapper.findById(existingAssetId);
            if (existing == null || existing.getUserId() == null || !existing.getUserId().equals(userId)) {
                throw new ApiException(HttpStatus.NOT_FOUND, "ASSET_NOT_FOUND", "图片资源不存在");
            }
            return toResponse(existing);
        }

        if (url.startsWith("data:image/")) {
            DecodedDataUrl decoded = decodeDataUrl(url);
            return uploadBytes(userId, canvasId, "imported" + extensionFor("imported", decoded.mimeType()), decoded.mimeType(), decoded.bytes());
        }

        URI uri;
        try {
            uri = URI.create(url);
        } catch (IllegalArgumentException exception) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "ASSET_URL_INVALID", "图片地址无效");
        }
        String scheme = uri.getScheme() == null ? "" : uri.getScheme().toLowerCase(Locale.ROOT);
        if (!"http".equals(scheme) && !"https".equals(scheme)) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "ASSET_URL_UNSUPPORTED", "只支持 http/https 图片地址");
        }

        HttpResponse<byte[]> response = downloadImageBytes(uri);
        if (response.statusCode() < 200 || response.statusCode() >= 300) {
            throw new ApiException(HttpStatus.BAD_GATEWAY, "ASSET_DOWNLOAD_FAILED", "下载生成图片失败");
        }
        byte[] body = response.body() == null ? new byte[0] : response.body();
        if (body.length == 0) {
            throw new ApiException(HttpStatus.BAD_GATEWAY, "ASSET_DOWNLOAD_FAILED", "下载生成图片失败");
        }
        if (body.length > IMAGE_IMPORT_MAX_BYTES) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "ASSET_TOO_LARGE", "生成图片过大，无法保存");
        }

        String contentType = response.headers().firstValue(org.springframework.http.HttpHeaders.CONTENT_TYPE).orElse("");
        if (contentType.contains(";")) {
            contentType = contentType.substring(0, contentType.indexOf(';')).trim();
        }
        String filename = filenameFromUri(uri);
        return uploadBytes(userId, canvasId, filename, contentType, body);
    }

    private HttpResponse<byte[]> downloadImageBytes(URI uri) {
        List<URI> candidates = new ArrayList<>();
        candidates.add(uri);
        URI dockerHostUri = rewriteLocalhostForDocker(uri);
        if (dockerHostUri != null) {
            candidates.add(dockerHostUri);
        }

        InterruptedException interrupted = null;
        HttpResponse<byte[]> lastResponse = null;
        for (URI candidate : candidates) {
            HttpRequest request = HttpRequest.newBuilder(candidate)
                    .timeout(IMAGE_IMPORT_TIMEOUT)
                    .header("Accept", "image/*,application/octet-stream;q=0.9,*/*;q=0.8")
                    .GET()
                    .build();
            try {
                HttpResponse<byte[]> response = httpClient.send(request, HttpResponse.BodyHandlers.ofByteArray());
                if (response.statusCode() >= 200 && response.statusCode() < 300) {
                    return response;
                }
                lastResponse = response;
            } catch (InterruptedException exception) {
                interrupted = exception;
                Thread.currentThread().interrupt();
                break;
            } catch (Exception ignored) {
            }
        }
        if (interrupted != null) {
            throw new ApiException(HttpStatus.BAD_GATEWAY, "ASSET_DOWNLOAD_FAILED", "下载生成图片被中断");
        }
        if (lastResponse != null) {
            return lastResponse;
        }
        throw new ApiException(HttpStatus.BAD_GATEWAY, "ASSET_DOWNLOAD_FAILED", "下载生成图片失败");
    }

    private URI rewriteLocalhostForDocker(URI uri) {
        String host = uri.getHost();
        if (host == null || (!"localhost".equalsIgnoreCase(host) && !"127.0.0.1".equals(host))) {
            return null;
        }
        try {
            return new URI(uri.getScheme(), uri.getUserInfo(), "host.docker.internal", uri.getPort(), uri.getPath(), uri.getQuery(), uri.getFragment());
        } catch (URISyntaxException exception) {
            return null;
        }
    }

    private UUID parseAssetIdFromUrl(String url) {
        if (url == null || url.isBlank()) {
            return null;
        }
        java.util.regex.Matcher matcher = java.util.regex.Pattern.compile("/api/assets/([0-9a-fA-F-]{36})(?:/(?:content|preview|thumbnail|view/[0-9]+))?(?:[?#].*)?$")
                .matcher(url.trim());
        if (!matcher.find()) {
            return null;
        }
        try {
            return UUID.fromString(matcher.group(1));
        } catch (IllegalArgumentException exception) {
            return null;
        }
    }

    private DecodedDataUrl decodeDataUrl(String dataUrl) {
        int comma = dataUrl.indexOf(',');
        if (comma < 0) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "ASSET_URL_INVALID", "图片地址无效");
        }
        String header = dataUrl.substring("data:".length(), comma);
        String payload = dataUrl.substring(comma + 1);
        String mimeType = "image/png";
        String[] parts = header.split(";");
        if (parts.length > 0 && !parts[0].isBlank()) {
            mimeType = parts[0].trim().toLowerCase(Locale.ROOT);
        }
        try {
            byte[] bytes = java.util.Base64.getDecoder().decode(payload);
            return new DecodedDataUrl(bytes, mimeType);
        } catch (IllegalArgumentException exception) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "ASSET_URL_INVALID", "图片地址无效");
        }
    }

    private String filenameFromUri(URI uri) {
        String path = uri.getPath();
        if (path == null || path.isBlank()) {
            return "generated-image.png";
        }
        int slash = path.lastIndexOf('/');
        String name = slash >= 0 ? path.substring(slash + 1) : path;
        return name.isBlank() ? "generated-image.png" : name;
    }

    private record DecodedDataUrl(byte[] bytes, String mimeType) {
    }

    private static MinioClient createMinioClient(String endpoint, String accessKey, String secretKey) {
        MinioEndpoint minioEndpoint = parseMinioEndpoint(endpoint);
        return MinioClient.builder()
                .endpoint(minioEndpoint.host(), minioEndpoint.port(), minioEndpoint.secure())
                .credentials(stripWrappingBackticks(accessKey), stripWrappingBackticks(secretKey))
                .build();
    }

    private static MinioEndpoint parseMinioEndpoint(String endpoint) {
        String normalizedEndpoint = stripWrappingBackticks(endpoint).trim();
        if (normalizedEndpoint.isEmpty()) {
            throw new IllegalArgumentException("MinIO endpoint is empty");
        }

        String uriText = normalizedEndpoint.matches("^[a-zA-Z][a-zA-Z0-9+.-]*://.*")
                ? normalizedEndpoint
                : "http://" + normalizedEndpoint;
        try {
            URI uri = new URI(uriText);
            if (uri.getHost() == null || uri.getHost().isBlank()) {
                throw new IllegalArgumentException("MinIO endpoint host is empty");
            }
            boolean secure = "https".equalsIgnoreCase(uri.getScheme());
            int port = uri.getPort() > 0 ? uri.getPort() : (secure ? 443 : 80);
            return new MinioEndpoint(uri.getHost(), port, secure);
        } catch (URISyntaxException exception) {
            throw new IllegalArgumentException("Invalid MinIO endpoint", exception);
        }
    }

    private static String stripWrappingBackticks(String value) {
        if (value == null) {
            return "";
        }
        String trimmedValue = value.trim();
        if (trimmedValue.length() >= 2 && trimmedValue.startsWith("`") && trimmedValue.endsWith("`")) {
            return trimmedValue.substring(1, trimmedValue.length() - 1);
        }
        return trimmedValue;
    }

    private record MinioEndpoint(String host, int port, boolean secure) {
    }

    public AssetContent getContent(UUID assetId) {
        AssetEntity entity = assetMapper.findById(assetId);
        if (entity == null) {
            throw new ApiException(HttpStatus.NOT_FOUND, "ASSET_NOT_FOUND", "图片资源不存在");
        }

        return openContent(entity);
    }

    public AssetContent getContentForUser(UUID userId, UUID assetId) {
        return openContent(requireUserAsset(userId, assetId));
    }

    public AssetContent getModelInputContentForUser(UUID userId, UUID assetId) {
        AssetEntity entity = requireUserAsset(userId, assetId);
        byte[] originalBytes = readOriginalBytes(entity);
        PreparedImageContent preparedImage = prepareModelInputImage(originalBytes, entity.getMimeType());
        return new AssetContent(entity, new ByteArrayInputStream(preparedImage.bytes()), preparedImage.contentType());
    }

    @Transactional
    public AssetDtos.AssetResponse rotate(UUID userId, UUID assetId, String directionValue, Integer quarterTurnsValue) {
        int quarterTurns = normalizeQuarterTurns(quarterTurnsValue != null
                ? quarterTurnsValue
                : AssetRotationDirection.from(directionValue).quarterTurns);
        AssetEntity entity = requireUserAsset(userId, assetId);
        if (quarterTurns == 0) {
            return toResponse(entity);
        }

        byte[] originalBytes = readOriginalBytes(entity);
        BufferedImage originalImage = readImage(originalBytes);
        if (originalImage == null) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "ASSET_ROTATE_UNSUPPORTED", "当前图片格式暂不支持旋转");
        }

        BufferedImage rotatedImage = rotateImage(originalImage, quarterTurns);
        EncodedImage encodedOriginal = encodeOriginalImage(rotatedImage, entity.getMimeType());

        try (InputStream inputStream = new ByteArrayInputStream(encodedOriginal.bytes())) {
            minioClient.putObject(PutObjectArgs.builder()
                    .bucket(properties.minio().bucket())
                    .object(entity.getObjectKey())
                    .stream(inputStream, encodedOriginal.bytes().length, -1)
                    .contentType(encodedOriginal.contentType())
                    .build());
            uploadImageVariants(entity.getObjectKey(), rotatedImage);
        } catch (Exception exception) {
            log.warn("[asset] rotate failed assetId={} bucket={} objectKey={} error={}",
                    entity.getId(), properties.minio().bucket(), entity.getObjectKey(), exception.getMessage());
            throw new ApiException(HttpStatus.BAD_GATEWAY, "ASSET_ROTATE_FAILED", "旋转图片资源失败");
        }

        entity.setMimeType(encodedOriginal.contentType());
        entity.setSizeBytes(encodedOriginal.bytes().length);
        entity.setWidth(rotatedImage.getWidth());
        entity.setHeight(rotatedImage.getHeight());
        if (assetMapper.updateAssetMetadata(entity) != 1) {
            throw new ApiException(HttpStatus.NOT_FOUND, "ASSET_NOT_FOUND", "图片资源不存在");
        }

        return toResponse(assetMapper.findById(assetId));
    }

    public PreparedImageContent prepareModelInputImage(byte[] originalBytes, String contentType) {
        String normalizedContentType = normalizeMimeType(contentType);
        BufferedImage image = readImage(originalBytes);
        if (image == null) {
            return new PreparedImageContent(originalBytes, normalizedContentType);
        }

        try {
            EncodedImage encoded = encodeJpegOrPngVariant(image, MODEL_INPUT_MAX_SIDE, MODEL_INPUT_JPEG_QUALITY);
            boolean resized = Math.max(image.getWidth(), image.getHeight()) > MODEL_INPUT_MAX_SIDE;
            boolean needsCompatibilityConversion = "image/webp".equals(normalizedContentType);
            if (resized || needsCompatibilityConversion || encoded.bytes().length < originalBytes.length) {
                return new PreparedImageContent(encoded.bytes(), encoded.contentType());
            }
        } catch (IOException ignored) {
        }

        return new PreparedImageContent(originalBytes, normalizedContentType);
    }

    @Transactional
    public int purgeAllUserAssetsAndObjects() {
        List<AssetEntity> assets = assetMapper.findAllUserOwnedAssets();
        int deletedObjectCount = 0;
        for (AssetEntity asset : assets) {
            deletedObjectCount += removeAssetObjects(asset);
        }
        assetMapper.deleteAllUserOwnedAssets();
        return deletedObjectCount;
    }

    private AssetEntity requireUserAsset(UUID userId, UUID assetId) {
        AssetEntity entity = assetMapper.findById(assetId);
        if (entity == null || !userId.equals(entity.getUserId())) {
            throw new ApiException(HttpStatus.NOT_FOUND, "ASSET_NOT_FOUND", "图片资源不存在");
        }

        return entity;
    }

    private AssetContent openContent(AssetEntity entity) {
        try {
            GetObjectResponse object = minioClient.getObject(GetObjectArgs.builder()
                    .bucket(properties.minio().bucket())
                    .object(entity.getObjectKey())
                    .build());
            return new AssetContent(entity, object, entity.getMimeType());
        } catch (Exception exception) {
            log.warn("[asset] read failed assetId={} bucket={} objectKey={} error={}",
                    entity.getId(), properties.minio().bucket(), entity.getObjectKey(), exception.getMessage());
            throw new ApiException(HttpStatus.BAD_GATEWAY, "ASSET_READ_FAILED", "读取图片资源失败");
        }
    }

    private byte[] readOriginalBytes(AssetEntity entity) {
        try (GetObjectResponse object = minioClient.getObject(GetObjectArgs.builder()
                .bucket(properties.minio().bucket())
                .object(entity.getObjectKey())
                .build())) {
            return object.readAllBytes();
        } catch (Exception exception) {
            log.warn("[asset] read original failed assetId={} bucket={} objectKey={} error={}",
                    entity.getId(), properties.minio().bucket(), entity.getObjectKey(), exception.getMessage());
            throw new ApiException(HttpStatus.BAD_GATEWAY, "ASSET_READ_FAILED", "读取图片资源失败");
        }
    }

    public AssetContent getPreview(UUID assetId) {
        return getVariantContent(assetId, "preview", PREVIEW_MAX_SIDE, PREVIEW_WEBP_QUALITY);
    }

    public AssetContent getThumbnail(UUID assetId) {
        return getVariantContent(assetId, "thumbnail", THUMBNAIL_MAX_SIDE, THUMBNAIL_WEBP_QUALITY);
    }

    public AssetContent getCanvasView(UUID assetId, int requestedWidth) {
        int width = CanvasViewVariantPolicy.normalizeWidth(requestedWidth);
        return getVariantContent(
                assetId,
                CanvasViewVariantPolicy.variantName(width),
                width,
                CANVAS_VIEW_WEBP_QUALITY,
                ResizeMode.MAX_WIDTH
        );
    }

    private void validateImage(MultipartFile file) {
        if (file == null || file.isEmpty()) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "EMPTY_FILE", "请上传图片文件");
        }

        String mimeType = normalizeMimeType(file.getContentType());
        if (!mimeType.startsWith("image/")) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "UNSUPPORTED_FILE_TYPE", "只支持图片文件");
        }
    }

    private BufferedImage readImage(byte[] fileBytes) {
        try (InputStream inputStream = new ByteArrayInputStream(fileBytes)) {
            return ImageIO.read(inputStream);
        } catch (IOException exception) {
            return null;
        }
    }

    private AssetDtos.AssetResponse toResponse(AssetEntity entity) {
        return new AssetDtos.AssetResponse(
                entity.getId(),
                entity.getCanvasId(),
                entity.getUserId(),
                entity.getUrlPath(),
                previewUrlPath(entity.getId()),
                thumbnailUrlPath(entity.getId()),
                entity.getOriginalFilename(),
                entity.getMimeType(),
                entity.getSizeBytes(),
                entity.getWidth(),
                entity.getHeight(),
                entity.getCreatedAt()
        );
    }

    private String normalizeMimeType(String contentType) {
        if (contentType == null || contentType.isBlank()) {
            return "application/octet-stream";
        }
        return contentType.trim().toLowerCase(Locale.ROOT);
    }

    private String normalizeFilename(String filename) {
        if (filename == null || filename.isBlank()) {
            return "canvas-image";
        }
        String normalized = filename.replaceAll("[\\\\/\\r\\n\\t]", "_").trim();
        return normalized.length() > 255 ? normalized.substring(normalized.length() - 255) : normalized;
    }

    private String extensionFor(String filename, String mimeType) {
        int dotIndex = filename.lastIndexOf('.');
        if (dotIndex >= 0 && dotIndex < filename.length() - 1) {
            return filename.substring(dotIndex).toLowerCase(Locale.ROOT);
        }
        return switch (mimeType) {
            case "image/jpeg", "image/jpg" -> ".jpg";
            case "image/webp" -> ".webp";
            case "image/gif" -> ".gif";
            case "image/svg+xml" -> ".svg";
            default -> ".png";
        };
    }

    private void uploadImageVariants(String objectKey, BufferedImage image) throws Exception {
        if (image == null) return;
        uploadImageVariant(variantObjectKey(objectKey, "preview", WEBP_FORMAT), image, PREVIEW_MAX_SIDE, PREVIEW_WEBP_QUALITY, ResizeMode.MAX_SIDE);
        uploadImageVariant(variantObjectKey(objectKey, "thumbnail", WEBP_FORMAT), image, THUMBNAIL_MAX_SIDE, THUMBNAIL_WEBP_QUALITY, ResizeMode.MAX_SIDE);
        for (int width : CanvasViewVariantPolicy.WIDTH_TIERS) {
            uploadImageVariant(
                    variantObjectKey(objectKey, CanvasViewVariantPolicy.variantName(width), WEBP_FORMAT),
                    image,
                    width,
                    CANVAS_VIEW_WEBP_QUALITY,
                    ResizeMode.MAX_WIDTH
            );
        }
    }

    private void uploadImageVariant(
            String objectKey,
            BufferedImage source,
            int maxDimension,
            float webpQuality,
            ResizeMode resizeMode
    ) throws Exception {
        EncodedImage encoded = encodeWebpVariant(source, maxDimension, webpQuality, resizeMode);
        try (InputStream inputStream = new ByteArrayInputStream(encoded.bytes())) {
            minioClient.putObject(PutObjectArgs.builder()
                    .bucket(properties.minio().bucket())
                    .object(objectKey)
                    .stream(inputStream, encoded.bytes().length, -1)
                    .contentType(encoded.contentType())
                    .build());
        }
    }

    private AssetContent getVariantContent(UUID assetId, String variant, int maxSide, float webpQuality) {
        return getVariantContent(assetId, variant, maxSide, webpQuality, ResizeMode.MAX_SIDE);
    }

    private AssetContent getVariantContent(
            UUID assetId,
            String variant,
            int maxDimension,
            float webpQuality,
            ResizeMode resizeMode
    ) {
        AssetEntity entity = assetMapper.findById(assetId);
        if (entity == null) {
            throw new ApiException(HttpStatus.NOT_FOUND, "ASSET_NOT_FOUND", "图片资源不存在");
        }

        AssetContent webpContent = tryOpenVariant(entity, variantObjectKey(entity.getObjectKey(), variant, WEBP_FORMAT));
        if (webpContent != null) {
            return webpContent;
        }

        AssetContent generatedWebpContent = generateVariantFromOriginal(entity, variant, maxDimension, webpQuality, resizeMode);
        if (generatedWebpContent != null) {
            return generatedWebpContent;
        }

        for (String objectKey : List.of(
                variantObjectKey(entity.getObjectKey(), variant, "jpg"),
                variantObjectKey(entity.getObjectKey(), variant, "png")
        )) {
            AssetContent legacyContent = tryOpenVariant(entity, objectKey);
            if (legacyContent != null) return legacyContent;
        }

        return getContent(entity.getId());
    }

    private AssetContent tryOpenVariant(AssetEntity entity, String objectKey) {
        try {
            GetObjectResponse object = minioClient.getObject(GetObjectArgs.builder()
                    .bucket(properties.minio().bucket())
                    .object(objectKey)
                    .build());
            return new AssetContent(entity, object, variantContentType(objectKey));
        } catch (Exception ignored) {
            return null;
        }
    }

    private AssetContent generateVariantFromOriginal(
            AssetEntity entity,
            String variant,
            int maxDimension,
            float webpQuality,
            ResizeMode resizeMode
    ) {
        try (GetObjectResponse object = minioClient.getObject(GetObjectArgs.builder()
                .bucket(properties.minio().bucket())
                .object(entity.getObjectKey())
                .build())) {
            BufferedImage image = ImageIO.read(object);
            if (image == null) {
                return null;
            }
            EncodedImage encoded = encodeWebpVariant(image, maxDimension, webpQuality, resizeMode);
            String generatedObjectKey = variantObjectKey(entity.getObjectKey(), variant, WEBP_FORMAT);
            try (InputStream inputStream = new ByteArrayInputStream(encoded.bytes())) {
                minioClient.putObject(PutObjectArgs.builder()
                        .bucket(properties.minio().bucket())
                        .object(generatedObjectKey)
                        .stream(inputStream, encoded.bytes().length, -1)
                        .contentType(encoded.contentType())
                        .build());
            }
            return new AssetContent(entity, new ByteArrayInputStream(encoded.bytes()), encoded.contentType());
        } catch (Exception exception) {
            return null;
        }
    }

    private EncodedImage encodeWebpVariant(
            BufferedImage source,
            int maxDimension,
            float webpQuality,
            ResizeMode resizeMode
    ) throws IOException {
        BufferedImage resized = switch (resizeMode) {
            case MAX_SIDE -> resizeToMaxSide(source, maxDimension);
            case MAX_WIDTH -> resizeToMaxWidth(source, maxDimension);
        };
        ByteArrayOutputStream outputStream = new ByteArrayOutputStream();
        writeCompressedImage(resized, WEBP_FORMAT, outputStream, webpQuality);
        return new EncodedImage(outputStream.toByteArray(), WEBP_CONTENT_TYPE);
    }

    private EncodedImage encodeJpegOrPngVariant(BufferedImage source, int maxSide, float jpegQuality) throws IOException {
        BufferedImage resized = resizeToMaxSide(source, maxSide);
        boolean hasAlpha = resized.getColorModel().hasAlpha();
        String format = hasAlpha ? "png" : "jpg";
        String contentType = hasAlpha ? "image/png" : "image/jpeg";
        ByteArrayOutputStream outputStream = new ByteArrayOutputStream();

        if (hasAlpha) {
            ImageIO.write(resized, format, outputStream);
        } else {
            writeJpeg(resized, outputStream, jpegQuality);
        }

        return new EncodedImage(outputStream.toByteArray(), contentType);
    }

    private EncodedImage encodeOriginalImage(BufferedImage source, String contentType) {
        String normalizedContentType = normalizeMimeType(contentType);
        ByteArrayOutputStream outputStream = new ByteArrayOutputStream();
        try {
            switch (normalizedContentType) {
                case "image/jpeg", "image/jpg" -> {
                    writeJpeg(source, outputStream, MODEL_INPUT_JPEG_QUALITY);
                    return new EncodedImage(outputStream.toByteArray(), "image/jpeg");
                }
                case "image/png" -> {
                    ImageIO.write(source, "png", outputStream);
                    return new EncodedImage(outputStream.toByteArray(), "image/png");
                }
                case "image/webp" -> {
                    writeCompressedImage(source, WEBP_FORMAT, outputStream, CANVAS_VIEW_WEBP_QUALITY);
                    return new EncodedImage(outputStream.toByteArray(), WEBP_CONTENT_TYPE);
                }
                default -> throw new ApiException(HttpStatus.BAD_REQUEST, "ASSET_ROTATE_UNSUPPORTED", "当前图片格式暂不支持旋转");
            }
        } catch (IOException exception) {
            throw new ApiException(HttpStatus.BAD_GATEWAY, "ASSET_ROTATE_FAILED", "旋转图片资源失败");
        }
    }

    private BufferedImage rotateImage(BufferedImage source, int quarterTurns) {
        int normalizedQuarterTurns = normalizeQuarterTurns(quarterTurns);
        if (normalizedQuarterTurns == 0) {
            return source;
        }
        boolean swapsDimensions = normalizedQuarterTurns % 2 == 1;
        int targetType = source.getColorModel().hasAlpha()
                ? BufferedImage.TYPE_INT_ARGB
                : BufferedImage.TYPE_INT_RGB;
        BufferedImage target = new BufferedImage(
                swapsDimensions ? source.getHeight() : source.getWidth(),
                swapsDimensions ? source.getWidth() : source.getHeight(),
                targetType
        );
        Graphics2D graphics = target.createGraphics();
        try {
            if (!source.getColorModel().hasAlpha()) {
                graphics.setColor(Color.WHITE);
                graphics.fillRect(0, 0, target.getWidth(), target.getHeight());
            }
            graphics.setRenderingHint(RenderingHints.KEY_INTERPOLATION, RenderingHints.VALUE_INTERPOLATION_BICUBIC);
            graphics.setRenderingHint(RenderingHints.KEY_RENDERING, RenderingHints.VALUE_RENDER_QUALITY);
            graphics.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            graphics.translate(target.getWidth() / 2.0, target.getHeight() / 2.0);
            graphics.rotate(normalizedQuarterTurns * Math.PI / 2);
            graphics.drawImage(source, -source.getWidth() / 2, -source.getHeight() / 2, null);
        } finally {
            graphics.dispose();
        }
        return target;
    }

    private int normalizeQuarterTurns(int quarterTurns) {
        return Math.floorMod(quarterTurns, 4);
    }

    private BufferedImage resizeToMaxSide(BufferedImage source, int maxSide) {
        int sourceWidth = source.getWidth();
        int sourceHeight = source.getHeight();
        int longestSide = Math.max(sourceWidth, sourceHeight);
        if (longestSide <= 0) return source;

        double scale = Math.min(1.0, (double) maxSide / longestSide);
        int targetWidth = Math.max(1, (int) Math.round(sourceWidth * scale));
        int targetHeight = Math.max(1, (int) Math.round(sourceHeight * scale));
        return resizeToDimensions(source, targetWidth, targetHeight);
    }

    private BufferedImage resizeToMaxWidth(BufferedImage source, int maxWidth) {
        int sourceWidth = source.getWidth();
        int sourceHeight = source.getHeight();
        if (sourceWidth <= 0) return source;

        double scale = Math.min(1.0, (double) maxWidth / sourceWidth);
        int targetWidth = Math.max(1, (int) Math.round(sourceWidth * scale));
        int targetHeight = Math.max(1, (int) Math.round(sourceHeight * scale));
        return resizeToDimensions(source, targetWidth, targetHeight);
    }

    private BufferedImage resizeToDimensions(BufferedImage source, int targetWidth, int targetHeight) {
        int imageType = source.getColorModel().hasAlpha()
                ? BufferedImage.TYPE_INT_ARGB
                : BufferedImage.TYPE_INT_RGB;
        BufferedImage target = new BufferedImage(targetWidth, targetHeight, imageType);
        Graphics2D graphics = target.createGraphics();
        try {
            if (!source.getColorModel().hasAlpha()) {
                graphics.setColor(Color.WHITE);
                graphics.fillRect(0, 0, targetWidth, targetHeight);
            }
            graphics.setRenderingHint(RenderingHints.KEY_INTERPOLATION, RenderingHints.VALUE_INTERPOLATION_BICUBIC);
            graphics.setRenderingHint(RenderingHints.KEY_RENDERING, RenderingHints.VALUE_RENDER_QUALITY);
            graphics.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            graphics.drawImage(source, 0, 0, targetWidth, targetHeight, null);
        } finally {
            graphics.dispose();
        }
        return target;
    }

    private void writeJpeg(BufferedImage image, ByteArrayOutputStream outputStream, float quality) throws IOException {
        writeCompressedImage(image, "jpg", outputStream, quality);
    }

    private void writeCompressedImage(
            BufferedImage image,
            String format,
            ByteArrayOutputStream outputStream,
            float quality
    ) throws IOException {
        Iterator<ImageWriter> writers = ImageIO.getImageWritersByFormatName(format);
        if (!writers.hasNext()) {
            if (!ImageIO.write(image, format, outputStream)) {
                throw new IOException("缺少 %s 图片编码器".formatted(format));
            }
            return;
        }

        ImageWriter writer = writers.next();
        try (ImageOutputStream imageOutputStream = ImageIO.createImageOutputStream(outputStream)) {
            writer.setOutput(imageOutputStream);
            ImageWriteParam writeParam = writer.getDefaultWriteParam();
            if (writeParam.canWriteCompressed()) {
                writeParam.setCompressionMode(ImageWriteParam.MODE_EXPLICIT);
                String[] compressionTypes = writeParam.getCompressionTypes();
                if (compressionTypes != null && compressionTypes.length > 0) {
                    writeParam.setCompressionType(preferredCompressionType(compressionTypes));
                }
                writeParam.setCompressionQuality(quality);
            }
            writer.write(null, new javax.imageio.IIOImage(image, null, null), writeParam);
        } finally {
            writer.dispose();
        }
    }

    private String preferredCompressionType(String[] compressionTypes) {
        for (String compressionType : compressionTypes) {
            if ("Lossy".equalsIgnoreCase(compressionType) || "JPEG".equalsIgnoreCase(compressionType)) {
                return compressionType;
            }
        }
        return compressionTypes[0];
    }

    private String variantObjectKey(String objectKey, String variant, String extension) {
        return "%s.%s.%s".formatted(objectKey, variant, extension);
    }

    private int removeAssetObjects(AssetEntity asset) {
        if (asset == null || asset.getObjectKey() == null || asset.getObjectKey().isBlank()) {
            return 0;
        }

        int removed = 0;
        for (String objectKey : buildAllObjectKeys(asset.getObjectKey())) {
            try {
                minioClient.removeObject(RemoveObjectArgs.builder()
                        .bucket(properties.minio().bucket())
                        .object(objectKey)
                        .build());
                removed += 1;
            } catch (Exception exception) {
                log.warn("[asset] purge remove object failed assetId={} objectKey={} error={}",
                        asset.getId(), objectKey, exception.getMessage());
            }
        }
        return removed;
    }

    private List<String> buildAllObjectKeys(String originalObjectKey) {
        List<String> objectKeys = new ArrayList<>();
        objectKeys.add(originalObjectKey);
        objectKeys.add(variantObjectKey(originalObjectKey, "preview", WEBP_FORMAT));
        objectKeys.add(variantObjectKey(originalObjectKey, "thumbnail", WEBP_FORMAT));
        for (int width : CanvasViewVariantPolicy.WIDTH_TIERS) {
            objectKeys.add(variantObjectKey(originalObjectKey, CanvasViewVariantPolicy.variantName(width), WEBP_FORMAT));
        }
        return objectKeys;
    }

    private String variantContentType(String objectKey) {
        if (objectKey.endsWith(".webp")) return WEBP_CONTENT_TYPE;
        return objectKey.endsWith(".png") ? "image/png" : "image/jpeg";
    }

    private String previewUrlPath(UUID assetId) {
        return "/api/assets/%s/preview".formatted(assetId);
    }

    private String thumbnailUrlPath(UUID assetId) {
        return "/api/assets/%s/thumbnail".formatted(assetId);
    }

    public record AssetContent(AssetEntity entity, InputStream stream, String contentType) {
    }

    public record PreparedImageContent(byte[] bytes, String contentType) {
    }

    private record ImageSize(Integer width, Integer height) {
    }

    private record EncodedImage(byte[] bytes, String contentType) {
    }

    private enum ResizeMode {
        MAX_SIDE,
        MAX_WIDTH
    }

    private enum AssetRotationDirection {
        LEFT(-1),
        RIGHT(1);

        private final int quarterTurns;

        AssetRotationDirection(int quarterTurns) {
            this.quarterTurns = quarterTurns;
        }

        private static AssetRotationDirection from(String value) {
            if ("left".equalsIgnoreCase(value)) {
                return LEFT;
            }
            if ("right".equalsIgnoreCase(value)) {
                return RIGHT;
            }
            throw new ApiException(HttpStatus.BAD_REQUEST, "INVALID_ROTATION_DIRECTION", "请选择向左或向右旋转");
        }
    }
}
