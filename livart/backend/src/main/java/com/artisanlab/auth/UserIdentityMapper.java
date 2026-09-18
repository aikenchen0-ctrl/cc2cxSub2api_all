package com.artisanlab.auth;

import org.apache.ibatis.annotations.Insert;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Select;
import org.apache.ibatis.annotations.Update;

@Mapper
public interface UserIdentityMapper {
    @Select("""
            SELECT id, user_id, provider, subject, provider_username, created_at, updated_at
            FROM artisan_user_identities
            WHERE provider = #{provider} AND subject = #{subject}
            """)
    UserIdentityEntity findByProviderAndSubject(
            @Param("provider") String provider,
            @Param("subject") String subject
    );

    @Insert("""
            INSERT INTO artisan_user_identities (id, user_id, provider, subject, provider_username, created_at, updated_at)
            VALUES (#{id}, #{userId}, #{provider}, #{subject}, #{providerUsername}, NOW(), NOW())
            """)
    void insertIdentity(UserIdentityEntity entity);

    @Update("""
            UPDATE artisan_user_identities
            SET provider_username = #{providerUsername}, updated_at = NOW()
            WHERE id = #{id}
            """)
    void updateIdentity(UserIdentityEntity entity);
}
