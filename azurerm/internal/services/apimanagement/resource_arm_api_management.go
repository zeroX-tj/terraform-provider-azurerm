package apimanagement

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/apimanagement/mgmt/2019-12-01/apimanagement"
	"github.com/hashicorp/go-azure-helpers/response"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/suppress"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/validate"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/clients"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/features"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tags"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/timeouts"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

var apimBackendProtocolSsl3 = "Microsoft.WindowsAzure.ApiManagement.Gateway.Security.Backend.Protocols.Ssl30"
var apimBackendProtocolTls10 = "Microsoft.WindowsAzure.ApiManagement.Gateway.Security.Backend.Protocols.Tls10"
var apimBackendProtocolTls11 = "Microsoft.WindowsAzure.ApiManagement.Gateway.Security.Backend.Protocols.Tls11"
var apimFrontendProtocolSsl3 = "Microsoft.WindowsAzure.ApiManagement.Gateway.Security.Protocols.Ssl30"
var apimFrontendProtocolTls10 = "Microsoft.WindowsAzure.ApiManagement.Gateway.Security.Protocols.Tls10"
var apimFrontendProtocolTls11 = "Microsoft.WindowsAzure.ApiManagement.Gateway.Security.Protocols.Tls11"
var apimTripleDesCiphers = "Microsoft.WindowsAzure.ApiManagement.Gateway.Security.Ciphers.TripleDes168"
var apimHttp2Protocol = "Microsoft.WindowsAzure.ApiManagement.Gateway.Protocols.Server.Http2"

func resourceArmApiManagementService() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmApiManagementServiceCreateUpdate,
		Read:   resourceArmApiManagementServiceRead,
		Update: resourceArmApiManagementServiceCreateUpdate,
		Delete: resourceArmApiManagementServiceDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(60 * time.Minute),
			Delete: schema.DefaultTimeout(60 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"name": azure.SchemaApiManagementName(),

			"resource_group_name": azure.SchemaResourceGroupName(),

			"location": azure.SchemaLocation(),

			"public_ip_addresses": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"private_ip_addresses": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"publisher_name": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validate.ApiManagementServicePublisherName,
			},

			"publisher_email": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validate.ApiManagementServicePublisherEmail,
			},

			"sku_name": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: azure.MinCapacitySkuNameInSlice([]string{
					string(apimanagement.SkuTypeDeveloper),
					string(apimanagement.SkuTypeBasic),
					string(apimanagement.SkuTypeStandard),
					string(apimanagement.SkuTypePremium),
				}, 1, false),
			},

			"identity": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": {
							Type:     schema.TypeString,
							Required: true,
							ValidateFunc: validation.StringInSlice([]string{
								"SystemAssigned",
							}, false),
						},
						"principal_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"tenant_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},

			"virtual_network_type": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(apimanagement.VirtualNetworkTypeNone),
					string(apimanagement.VirtualNetworkTypeExternal),
					string(apimanagement.VirtualNetworkTypeInternal),
				}, false),
			},

			"virtual_network_configuration": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"subnet_id": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
					},
				},
			},

			"notification_sender_email": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"additional_location": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"location": azure.SchemaLocation(),

						"gateway_regional_url": {
							Type:     schema.TypeString,
							Computed: true,
						},

						"public_ip_addresses": {
							Type: schema.TypeList,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
							Computed: true,
						},
					},
				},
			},

			"certificate": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 10,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"encoded_certificate": {
							Type:      schema.TypeString,
							Required:  true,
							Sensitive: true,
						},

						"certificate_password": {
							Type:      schema.TypeString,
							Required:  true,
							Sensitive: true,
						},

						"store_name": {
							Type:     schema.TypeString,
							Required: true,
							ValidateFunc: validation.StringInSlice([]string{
								string(apimanagement.CertificateAuthority),
								string(apimanagement.Root),
							}, false),
						},
					},
				},
			},

			"protocols": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enable_http2": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
					},
				},
			},

			"security": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enable_backend_ssl30": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"enable_backend_tls10": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"enable_backend_tls11": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},

						"enable_frontend_ssl30": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},

						"enable_frontend_tls10": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},

						"enable_frontend_tls11": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},

						"enable_triple_des_ciphers": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
					},
				},
			},

			"hostname_configuration": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"management": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Resource{
								Schema: apiManagementResourceHostnameSchema("management"),
							},
						},
						"portal": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Resource{
								Schema: apiManagementResourceHostnameSchema("portal"),
							},
						},
						"proxy": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Resource{
								Schema: apiManagementResourceHostnameProxySchema(),
							},
						},
						"scm": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Resource{
								Schema: apiManagementResourceHostnameSchema("scm"),
							},
						},
					},
				},
			},

			"policy": {
				Type:       schema.TypeList,
				Optional:   true,
				Computed:   true,
				MaxItems:   1,
				ConfigMode: schema.SchemaConfigModeAttr,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"xml_content": {
							Type:             schema.TypeString,
							Optional:         true,
							Computed:         true,
							ConflictsWith:    []string{"policy.0.xml_link"},
							DiffSuppressFunc: suppress.XmlDiff,
						},

						"xml_link": {
							Type:          schema.TypeString,
							Optional:      true,
							ConflictsWith: []string{"policy.0.xml_content"},
						},
					},
				},
			},

			"sign_in": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enabled": {
							Type:     schema.TypeBool,
							Required: true,
						},
					},
				},
			},

			"sign_up": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enabled": {
							Type:     schema.TypeBool,
							Required: true,
						},

						"terms_of_service": {
							Type:     schema.TypeList,
							Required: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"enabled": {
										Type:     schema.TypeBool,
										Required: true,
									},
									"consent_required": {
										Type:     schema.TypeBool,
										Required: true,
									},
									"text": {
										Type:     schema.TypeString,
										Optional: true,
									},
								},
							},
						},
					},
				},
			},

			"tags": tags.Schema(),

			"gateway_url": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"gateway_regional_url": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"portal_url": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"management_api_url": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"scm_url": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceArmApiManagementServiceCreateUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).ApiManagement.ServiceClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	sku := expandAzureRmApiManagementSkuName(d)

	log.Printf("[INFO] preparing arguments for API Management Service creation.")

	name := d.Get("name").(string)
	resourceGroup := d.Get("resource_group_name").(string)

	if features.ShouldResourcesBeImported() && d.IsNewResource() {
		existing, err := client.Get(ctx, resourceGroup, name)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for presence of existing API Management Service %q (Resource Group %q): %s", name, resourceGroup, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_api_management", *existing.ID)
		}
	}

	location := azure.NormalizeLocation(d.Get("location").(string))
	t := d.Get("tags").(map[string]interface{})

	publisherName := d.Get("publisher_name").(string)
	publisherEmail := d.Get("publisher_email").(string)
	notificationSenderEmail := d.Get("notification_sender_email").(string)
	virtualNetworkType := d.Get("virtual_network_type").(string)

	customProperties := expandApiManagementCustomProperties(d)
	certificates := expandAzureRmApiManagementCertificates(d)
	hostnameConfigurations := expandAzureRmApiManagementHostnameConfigurations(d)

	properties := apimanagement.ServiceResource{
		Location: utils.String(location),
		ServiceProperties: &apimanagement.ServiceProperties{
			PublisherName:          utils.String(publisherName),
			PublisherEmail:         utils.String(publisherEmail),
			CustomProperties:       customProperties,
			Certificates:           certificates,
			HostnameConfigurations: hostnameConfigurations,
		},
		Tags: tags.Expand(t),
		Sku:  sku,
	}

	if _, ok := d.GetOk("identity"); ok {
		properties.Identity = expandAzureRmApiManagementIdentity(d)
	}

	if _, ok := d.GetOk("additional_location"); ok {
		properties.ServiceProperties.AdditionalLocations = expandAzureRmApiManagementAdditionalLocations(d, sku)
	}

	if notificationSenderEmail != "" {
		properties.ServiceProperties.NotificationSenderEmail = &notificationSenderEmail
	}

	if virtualNetworkType != "" {
		properties.ServiceProperties.VirtualNetworkType = apimanagement.VirtualNetworkType(virtualNetworkType)

		if virtualNetworkType != string(apimanagement.VirtualNetworkTypeNone) {
			virtualNetworkConfiguration := expandAzureRmApiManagementVirtualNetworkConfigurations(d)
			if virtualNetworkConfiguration == nil {
				return fmt.Errorf("You must specify 'virtual_network_configuration' when 'virtual_network_type' is %q", virtualNetworkType)
			}
			properties.ServiceProperties.VirtualNetworkConfiguration = virtualNetworkConfiguration
		}
	}

	future, err := client.CreateOrUpdate(ctx, resourceGroup, name, properties)
	if err != nil {
		return fmt.Errorf("creating/updating API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("waiting for creation/update of API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	read, err := client.Get(ctx, resourceGroup, name)
	if err != nil {
		return fmt.Errorf("retrieving API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	if read.ID == nil {
		return fmt.Errorf("Cannot read ID for API Management Service %q (Resource Group %q)", name, resourceGroup)
	}

	d.SetId(*read.ID)

	signInSettingsRaw := d.Get("sign_in").([]interface{})
	signInSettings := expandApiManagementSignInSettings(signInSettingsRaw)
	signInClient := meta.(*clients.Client).ApiManagement.SignInClient
	if _, err := signInClient.CreateOrUpdate(ctx, resourceGroup, name, signInSettings, ""); err != nil {
		return fmt.Errorf(" setting Sign In settings for API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	signUpSettingsRaw := d.Get("sign_up").([]interface{})
	signUpSettings := expandApiManagementSignUpSettings(signUpSettingsRaw)
	signUpClient := meta.(*clients.Client).ApiManagement.SignUpClient
	if _, err := signUpClient.CreateOrUpdate(ctx, resourceGroup, name, signUpSettings, ""); err != nil {
		return fmt.Errorf(" setting Sign Up settings for API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	policyClient := meta.(*clients.Client).ApiManagement.PolicyClient
	policiesRaw := d.Get("policy").([]interface{})
	policy, err := expandApiManagementPolicies(policiesRaw)
	if err != nil {
		return err
	}

	if d.HasChange("policy") {
		// remove the existing policy
		if resp, err := policyClient.Delete(ctx, resourceGroup, name, ""); err != nil {
			if !utils.ResponseWasNotFound(resp) {
				return fmt.Errorf("removing Policies from API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
			}
		}

		// then add the new one, if it exists
		if policy != nil {
			if _, err := policyClient.CreateOrUpdate(ctx, resourceGroup, name, *policy, ""); err != nil {
				return fmt.Errorf(" setting Policies for API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
			}
		}
	}

	return resourceArmApiManagementServiceRead(d, meta)
}

func resourceArmApiManagementServiceRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).ApiManagement.ServiceClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resourceGroup := id.ResourceGroup
	name := id.Path["service"]

	resp, err := client.Get(ctx, resourceGroup, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("API Management Service %q was not found in Resource Group %q - removing from state!", name, resourceGroup)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("making Read request on API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	signInClient := meta.(*clients.Client).ApiManagement.SignInClient
	signInSettings, err := signInClient.Get(ctx, resourceGroup, name)
	if err != nil {
		return fmt.Errorf("retrieving Sign In Settings for API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	signUpClient := meta.(*clients.Client).ApiManagement.SignUpClient
	signUpSettings, err := signUpClient.Get(ctx, resourceGroup, name)
	if err != nil {
		return fmt.Errorf("retrieving Sign Up Settings for API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	policyClient := meta.(*clients.Client).ApiManagement.PolicyClient
	policy, err := policyClient.Get(ctx, resourceGroup, name, apimanagement.PolicyExportFormatXML)
	if err != nil {
		if !utils.ResponseWasNotFound(policy.Response) {
			return fmt.Errorf("retrieving Policy for API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
		}
	}

	d.Set("name", name)
	d.Set("resource_group_name", resourceGroup)

	if location := resp.Location; location != nil {
		d.Set("location", azure.NormalizeLocation(*location))
	}

	identity := flattenAzureRmApiManagementMachineIdentity(resp.Identity)
	if err := d.Set("identity", identity); err != nil {
		return fmt.Errorf("setting `identity`: %+v", err)
	}

	if props := resp.ServiceProperties; props != nil {
		d.Set("publisher_email", props.PublisherEmail)
		d.Set("publisher_name", props.PublisherName)
		d.Set("notification_sender_email", props.NotificationSenderEmail)
		d.Set("gateway_url", props.GatewayURL)
		d.Set("gateway_regional_url", props.GatewayRegionalURL)
		d.Set("portal_url", props.PortalURL)
		d.Set("management_api_url", props.ManagementAPIURL)
		d.Set("scm_url", props.ScmURL)
		d.Set("public_ip_addresses", props.PublicIPAddresses)
		d.Set("private_ip_addresses", props.PrivateIPAddresses)
		d.Set("virtual_network_type", props.VirtualNetworkType)

		if err := d.Set("security", flattenApiManagementSecurityCustomProperties(props.CustomProperties)); err != nil {
			return fmt.Errorf("setting `security`: %+v", err)
		}

		if err := d.Set("protocols", flattenApiManagementProtocolsCustomProperties(props.CustomProperties)); err != nil {
			return fmt.Errorf("setting `protocols`: %+v", err)
		}

		hostnameConfigs := flattenApiManagementHostnameConfigurations(props.HostnameConfigurations, d)
		if err := d.Set("hostname_configuration", hostnameConfigs); err != nil {
			return fmt.Errorf("setting `hostname_configuration`: %+v", err)
		}

		if err := d.Set("additional_location", flattenApiManagementAdditionalLocations(props.AdditionalLocations)); err != nil {
			return fmt.Errorf("setting `additional_location`: %+v", err)
		}

		if err := d.Set("virtual_network_configuration", flattenApiManagementVirtualNetworkConfiguration(props.VirtualNetworkConfiguration)); err != nil {
			return fmt.Errorf("Error setting `virtual_network_configuration`: %+v", err)
		}
	}

	if err := d.Set("sku_name", flattenApiManagementServiceSkuName(resp.Sku)); err != nil {
		return fmt.Errorf("setting `sku_name`: %+v", err)
	}

	if err := d.Set("sign_in", flattenApiManagementSignInSettings(signInSettings)); err != nil {
		return fmt.Errorf("setting `sign_in`: %+v", err)
	}

	if err := d.Set("sign_up", flattenApiManagementSignUpSettings(signUpSettings)); err != nil {
		return fmt.Errorf("setting `sign_up`: %+v", err)
	}

	if err := d.Set("policy", flattenApiManagementPolicies(d, policy)); err != nil {
		return fmt.Errorf("setting `policy`: %+v", err)
	}

	return tags.FlattenAndSet(d, resp.Tags)
}

func resourceArmApiManagementServiceDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).ApiManagement.ServiceClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	resourceGroup := id.ResourceGroup
	name := id.Path["service"]

	log.Printf("[DEBUG] Deleting API Management Service %q (Resource Grouo %q)", name, resourceGroup)
	future, err := client.Delete(ctx, resourceGroup, name)
	if err != nil {
		return fmt.Errorf("deleting API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		if !response.WasNotFound(future.Response()) {
			return fmt.Errorf("waiting for deletion of API Management Service %q (Resource Group %q): %+v", name, resourceGroup, err)
		}
	}

	return nil
}

func expandAzureRmApiManagementHostnameConfigurations(d *schema.ResourceData) *[]apimanagement.HostnameConfiguration {
	results := make([]apimanagement.HostnameConfiguration, 0)
	hostnameVs := d.Get("hostname_configuration").([]interface{})

	for _, hostnameRawVal := range hostnameVs {
		hostnameV := hostnameRawVal.(map[string]interface{})

		managementVs := hostnameV["management"].([]interface{})
		for _, managementV := range managementVs {
			v := managementV.(map[string]interface{})
			output := expandApiManagementCommonHostnameConfiguration(v, apimanagement.HostnameTypeManagement)
			results = append(results, output)
		}

		portalVs := hostnameV["portal"].([]interface{})
		for _, portalV := range portalVs {
			v := portalV.(map[string]interface{})
			output := expandApiManagementCommonHostnameConfiguration(v, apimanagement.HostnameTypePortal)
			results = append(results, output)
		}

		proxyVs := hostnameV["proxy"].([]interface{})
		for _, proxyV := range proxyVs {
			v := proxyV.(map[string]interface{})
			output := expandApiManagementCommonHostnameConfiguration(v, apimanagement.HostnameTypeProxy)
			if value, ok := v["default_ssl_binding"]; ok {
				output.DefaultSslBinding = utils.Bool(value.(bool))
			}
			results = append(results, output)
		}

		scmVs := hostnameV["scm"].([]interface{})
		for _, scmV := range scmVs {
			v := scmV.(map[string]interface{})
			output := expandApiManagementCommonHostnameConfiguration(v, apimanagement.HostnameTypeScm)
			results = append(results, output)
		}
	}

	return &results
}

func expandApiManagementCommonHostnameConfiguration(input map[string]interface{}, hostnameType apimanagement.HostnameType) apimanagement.HostnameConfiguration {
	encodedCertificate := input["certificate"].(string)
	certificatePassword := input["certificate_password"].(string)
	hostName := input["host_name"].(string)
	keyVaultId := input["key_vault_id"].(string)

	output := apimanagement.HostnameConfiguration{
		EncodedCertificate:  utils.String(encodedCertificate),
		CertificatePassword: utils.String(certificatePassword),
		HostName:            utils.String(hostName),
		KeyVaultID:          utils.String(keyVaultId),
		Type:                hostnameType,
	}

	if v, ok := input["negotiate_client_certificate"]; ok {
		output.NegotiateClientCertificate = utils.Bool(v.(bool))
	}

	return output
}

func flattenApiManagementHostnameConfigurations(input *[]apimanagement.HostnameConfiguration, d *schema.ResourceData) []interface{} {
	results := make([]interface{}, 0)
	if input == nil {
		return results
	}

	managementResults := make([]interface{}, 0)
	portalResults := make([]interface{}, 0)
	proxyResults := make([]interface{}, 0)
	scmResults := make([]interface{}, 0)

	for _, config := range *input {
		output := make(map[string]interface{})

		if config.HostName != nil {
			output["host_name"] = *config.HostName
		}

		if config.NegotiateClientCertificate != nil {
			output["negotiate_client_certificate"] = *config.NegotiateClientCertificate
		}

		if config.KeyVaultID != nil {
			output["key_vault_id"] = *config.KeyVaultID
		}

		// Iterate through old state to find sensitive props not returned by API.
		// This must be done in order to avoid state diffs.
		// NOTE: this information won't be available during times like Import, so this is a best-effort.
		existingHostnames := d.Get("hostname_configuration").([]interface{})
		if len(existingHostnames) > 0 {
			v := existingHostnames[0].(map[string]interface{})

			if valsRaw, ok := v[strings.ToLower(string(config.Type))]; ok {
				vals := valsRaw.([]interface{})
				for _, val := range vals {
					oldConfig := val.(map[string]interface{})

					if oldConfig["host_name"] == *config.HostName {
						output["certificate_password"] = oldConfig["certificate_password"]
						output["certificate"] = oldConfig["certificate"]
					}
				}
			}
		}

		switch strings.ToLower(string(config.Type)) {
		case strings.ToLower(string(apimanagement.HostnameTypeProxy)):
			// only set SSL binding for proxy types
			if config.DefaultSslBinding != nil {
				output["default_ssl_binding"] = *config.DefaultSslBinding
			}
			proxyResults = append(proxyResults, output)

		case strings.ToLower(string(apimanagement.HostnameTypeManagement)):
			managementResults = append(managementResults, output)

		case strings.ToLower(string(apimanagement.HostnameTypePortal)):
			portalResults = append(portalResults, output)

		case strings.ToLower(string(apimanagement.HostnameTypeScm)):
			scmResults = append(scmResults, output)
		}
	}

	return []interface{}{
		map[string]interface{}{
			"management": managementResults,
			"portal":     portalResults,
			"proxy":      proxyResults,
			"scm":        scmResults,
		},
	}
}

func expandAzureRmApiManagementCertificates(d *schema.ResourceData) *[]apimanagement.CertificateConfiguration {
	vs := d.Get("certificate").([]interface{})

	results := make([]apimanagement.CertificateConfiguration, 0)

	for _, v := range vs {
		config := v.(map[string]interface{})

		certBase64 := config["encoded_certificate"].(string)
		certificatePassword := config["certificate_password"].(string)
		storeName := apimanagement.StoreName(config["store_name"].(string))

		cert := apimanagement.CertificateConfiguration{
			EncodedCertificate:  utils.String(certBase64),
			CertificatePassword: utils.String(certificatePassword),
			StoreName:           storeName,
		}

		results = append(results, cert)
	}

	return &results
}

func expandAzureRmApiManagementAdditionalLocations(d *schema.ResourceData, sku *apimanagement.ServiceSkuProperties) *[]apimanagement.AdditionalLocation {
	inputLocations := d.Get("additional_location").([]interface{})

	additionalLocations := make([]apimanagement.AdditionalLocation, 0)

	for _, v := range inputLocations {
		config := v.(map[string]interface{})
		location := azure.NormalizeLocation(config["location"].(string))

		additionalLocation := apimanagement.AdditionalLocation{
			Location: utils.String(location),
			Sku:      sku,
		}

		additionalLocations = append(additionalLocations, additionalLocation)
	}

	return &additionalLocations
}

func flattenApiManagementAdditionalLocations(input *[]apimanagement.AdditionalLocation) []interface{} {
	results := make([]interface{}, 0)
	if input == nil {
		return results
	}

	for _, prop := range *input {
		output := make(map[string]interface{})

		if prop.Location != nil {
			output["location"] = azure.NormalizeLocation(*prop.Location)
		}

		if prop.PublicIPAddresses != nil {
			output["public_ip_addresses"] = *prop.PublicIPAddresses
		}

		if prop.GatewayRegionalURL != nil {
			output["gateway_regional_url"] = *prop.GatewayRegionalURL
		}

		results = append(results, output)
	}

	return results
}

func expandAzureRmApiManagementIdentity(d *schema.ResourceData) *apimanagement.ServiceIdentity {
	vs := d.Get("identity").([]interface{})
	if len(vs) == 0 {
		return nil
	}

	v := vs[0].(map[string]interface{})
	identityType := v["type"].(string)
	return &apimanagement.ServiceIdentity{
		Type: apimanagement.ApimIdentityType(identityType),
	}
}

func flattenAzureRmApiManagementMachineIdentity(identity *apimanagement.ServiceIdentity) []interface{} {
	if identity == nil {
		return make([]interface{}, 0)
	}

	result := make(map[string]interface{})

	result["type"] = string(identity.Type)

	if identity.PrincipalID != nil {
		result["principal_id"] = identity.PrincipalID.String()
	}

	if identity.TenantID != nil {
		result["tenant_id"] = identity.TenantID.String()
	}

	return []interface{}{result}
}

func expandAzureRmApiManagementSkuName(d *schema.ResourceData) *apimanagement.ServiceSkuProperties {
	vs := d.Get("sku_name").(string)

	if len(vs) == 0 {
		return nil
	}

	name, capacity, err := azure.SplitSku(vs)
	if err != nil {
		return nil
	}

	return &apimanagement.ServiceSkuProperties{
		Name:     apimanagement.SkuType(name),
		Capacity: utils.Int32(capacity),
	}
}

func flattenApiManagementServiceSkuName(input *apimanagement.ServiceSkuProperties) string {
	if input == nil {
		return ""
	}

	return fmt.Sprintf("%s_%d", string(input.Name), *input.Capacity)
}

func expandApiManagementCustomProperties(d *schema.ResourceData) map[string]*string {
	backendProtocolSsl3 := false
	backendProtocolTls10 := false
	backendProtocolTls11 := false
	frontendProtocolSsl3 := false
	frontendProtocolTls10 := false
	frontendProtocolTls11 := false
	tripleDesCiphers := false

	if vs := d.Get("security").([]interface{}); len(vs) > 0 {
		v := vs[0].(map[string]interface{})
		backendProtocolSsl3 = v["enable_backend_ssl30"].(bool)
		backendProtocolTls10 = v["enable_backend_tls10"].(bool)
		backendProtocolTls11 = v["enable_backend_tls11"].(bool)
		frontendProtocolSsl3 = v["enable_frontend_ssl30"].(bool)
		frontendProtocolTls10 = v["enable_frontend_tls10"].(bool)
		frontendProtocolTls11 = v["enable_frontend_tls11"].(bool)
		tripleDesCiphers = v["enable_triple_des_ciphers"].(bool)
	}

	customProperties := map[string]*string{
		apimBackendProtocolSsl3:   utils.String(strconv.FormatBool(backendProtocolSsl3)),
		apimBackendProtocolTls10:  utils.String(strconv.FormatBool(backendProtocolTls10)),
		apimBackendProtocolTls11:  utils.String(strconv.FormatBool(backendProtocolTls11)),
		apimFrontendProtocolSsl3:  utils.String(strconv.FormatBool(frontendProtocolSsl3)),
		apimFrontendProtocolTls10: utils.String(strconv.FormatBool(frontendProtocolTls10)),
		apimFrontendProtocolTls11: utils.String(strconv.FormatBool(frontendProtocolTls11)),
		apimTripleDesCiphers:      utils.String(strconv.FormatBool(tripleDesCiphers)),
	}

	if vp := d.Get("protocols").([]interface{}); len(vp) > 0 {
		if p, ok := d.GetOkExists("protocols.0.enable_http2"); ok {
			customProperties[apimHttp2Protocol] = utils.String(strconv.FormatBool(p.(bool)))
		}
	}

	return customProperties
}

func expandAzureRmApiManagementVirtualNetworkConfigurations(d *schema.ResourceData) *apimanagement.VirtualNetworkConfiguration {
	vs := d.Get("virtual_network_configuration").([]interface{})
	if len(vs) == 0 {
		return nil
	}

	v := vs[0].(map[string]interface{})
	subnetResourceId := v["subnet_id"].(string)

	return &apimanagement.VirtualNetworkConfiguration{
		SubnetResourceID: &subnetResourceId,
	}
}

func flattenApiManagementSecurityCustomProperties(input map[string]*string) []interface{} {
	output := make(map[string]interface{})

	output["enable_backend_ssl30"] = parseApiManagementNilableDictionary(input, apimBackendProtocolSsl3)
	output["enable_backend_tls10"] = parseApiManagementNilableDictionary(input, apimBackendProtocolTls10)
	output["enable_backend_tls11"] = parseApiManagementNilableDictionary(input, apimBackendProtocolTls11)
	output["enable_frontend_ssl30"] = parseApiManagementNilableDictionary(input, apimFrontendProtocolSsl3)
	output["enable_frontend_tls10"] = parseApiManagementNilableDictionary(input, apimFrontendProtocolTls10)
	output["enable_frontend_tls11"] = parseApiManagementNilableDictionary(input, apimFrontendProtocolTls11)
	output["enable_triple_des_ciphers"] = parseApiManagementNilableDictionary(input, apimTripleDesCiphers)

	return []interface{}{output}
}

func flattenApiManagementProtocolsCustomProperties(input map[string]*string) []interface{} {
	output := make(map[string]interface{})

	output["enable_http2"] = parseApiManagementNilableDictionary(input, apimHttp2Protocol)

	return []interface{}{output}
}

func flattenApiManagementVirtualNetworkConfiguration(input *apimanagement.VirtualNetworkConfiguration) []interface{} {
	if input == nil {
		return []interface{}{}
	}

	virtualNetworkConfiguration := make(map[string]interface{})

	if input.SubnetResourceID != nil {
		virtualNetworkConfiguration["subnet_id"] = *input.SubnetResourceID
	}

	return []interface{}{virtualNetworkConfiguration}
}

func apiManagementResourceHostnameSchema(schemaName string) map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"host_name": {
			Type:         schema.TypeString,
			Required:     true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"key_vault_id": {
			Type:         schema.TypeString,
			Optional:     true,
			ValidateFunc: azure.ValidateKeyVaultChildId,
			ConflictsWith: []string{
				fmt.Sprintf("hostname_configuration.0.%s.0.certificate", schemaName),
				fmt.Sprintf("hostname_configuration.0.%s.0.certificate_password", schemaName),
			},
		},

		"certificate": {
			Type:         schema.TypeString,
			Optional:     true,
			Sensitive:    true,
			ValidateFunc: validation.StringIsNotEmpty,
			ConflictsWith: []string{
				fmt.Sprintf("hostname_configuration.0.%s.0.key_vault_id", schemaName),
			},
		},

		"certificate_password": {
			Type:         schema.TypeString,
			Optional:     true,
			Sensitive:    true,
			ValidateFunc: validation.StringIsNotEmpty,
			ConflictsWith: []string{
				fmt.Sprintf("hostname_configuration.0.%s.0.key_vault_id", schemaName),
			},
		},

		"negotiate_client_certificate": {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  false,
		},
	}
}

func apiManagementResourceHostnameProxySchema() map[string]*schema.Schema {
	hostnameSchema := apiManagementResourceHostnameSchema("proxy")

	hostnameSchema["default_ssl_binding"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Computed: true, // Azure has certain logic to set this, which we cannot predict
	}

	return hostnameSchema
}

func parseApiManagementNilableDictionary(input map[string]*string, key string) bool {
	log.Printf("Parsing value for %q", key)

	v, ok := input[key]
	if !ok {
		log.Printf("%q was not found in the input - returning `false` as the default value", key)
		return false
	}

	val, err := strconv.ParseBool(*v)
	if err != nil {
		log.Printf(" parsing %q (key %q) as bool: %+v - assuming false", key, *v, err)
		return false
	}

	return val
}

func expandApiManagementSignInSettings(input []interface{}) apimanagement.PortalSigninSettings {
	enabled := false

	if len(input) > 0 {
		vs := input[0].(map[string]interface{})
		enabled = vs["enabled"].(bool)
	}

	return apimanagement.PortalSigninSettings{
		PortalSigninSettingProperties: &apimanagement.PortalSigninSettingProperties{
			Enabled: utils.Bool(enabled),
		},
	}
}

func flattenApiManagementSignInSettings(input apimanagement.PortalSigninSettings) []interface{} {
	enabled := false

	if props := input.PortalSigninSettingProperties; props != nil {
		if props.Enabled != nil {
			enabled = *props.Enabled
		}
	}

	return []interface{}{
		map[string]interface{}{
			"enabled": enabled,
		},
	}
}

func expandApiManagementSignUpSettings(input []interface{}) apimanagement.PortalSignupSettings {
	if len(input) == 0 {
		return apimanagement.PortalSignupSettings{
			PortalSignupSettingsProperties: &apimanagement.PortalSignupSettingsProperties{
				Enabled: utils.Bool(false),
				TermsOfService: &apimanagement.TermsOfServiceProperties{
					ConsentRequired: utils.Bool(false),
					Enabled:         utils.Bool(false),
					Text:            utils.String(""),
				},
			},
		}
	}

	vs := input[0].(map[string]interface{})

	props := apimanagement.PortalSignupSettingsProperties{
		Enabled: utils.Bool(vs["enabled"].(bool)),
	}

	termsOfServiceRaw := vs["terms_of_service"].([]interface{})
	if len(termsOfServiceRaw) > 0 {
		termsOfServiceVs := termsOfServiceRaw[0].(map[string]interface{})
		props.TermsOfService = &apimanagement.TermsOfServiceProperties{
			Enabled:         utils.Bool(termsOfServiceVs["enabled"].(bool)),
			ConsentRequired: utils.Bool(termsOfServiceVs["consent_required"].(bool)),
			Text:            utils.String(termsOfServiceVs["text"].(string)),
		}
	}

	return apimanagement.PortalSignupSettings{
		PortalSignupSettingsProperties: &props,
	}
}

func flattenApiManagementSignUpSettings(input apimanagement.PortalSignupSettings) []interface{} {
	enabled := false
	termsOfService := make([]interface{}, 0)

	if props := input.PortalSignupSettingsProperties; props != nil {
		if props.Enabled != nil {
			enabled = *props.Enabled
		}

		if tos := props.TermsOfService; tos != nil {
			output := make(map[string]interface{})

			if tos.Enabled != nil {
				output["enabled"] = *tos.Enabled
			}

			if tos.ConsentRequired != nil {
				output["consent_required"] = *tos.ConsentRequired
			}

			if tos.Text != nil {
				output["text"] = *tos.Text
			}

			termsOfService = append(termsOfService, output)
		}
	}

	return []interface{}{
		map[string]interface{}{
			"enabled":          enabled,
			"terms_of_service": termsOfService,
		},
	}
}

func expandApiManagementPolicies(input []interface{}) (*apimanagement.PolicyContract, error) {
	if len(input) == 0 {
		return nil, nil
	}

	vs := input[0].(map[string]interface{})
	xmlContent := vs["xml_content"].(string)
	xmlLink := vs["xml_link"].(string)

	if xmlContent != "" {
		return &apimanagement.PolicyContract{
			PolicyContractProperties: &apimanagement.PolicyContractProperties{
				Format: apimanagement.XML,
				Value:  utils.String(xmlContent),
			},
		}, nil
	}

	if xmlLink != "" {
		return &apimanagement.PolicyContract{
			PolicyContractProperties: &apimanagement.PolicyContractProperties{
				Format: apimanagement.XMLLink,
				Value:  utils.String(xmlLink),
			},
		}, nil
	}

	return nil, fmt.Errorf("Either `xml_content` or `xml_link` should be set if the `policy` block is defined.")
}

func flattenApiManagementPolicies(d *schema.ResourceData, input apimanagement.PolicyContract) []interface{} {
	xmlContent := ""
	if props := input.PolicyContractProperties; props != nil {
		if props.Value != nil {
			xmlContent = *props.Value
		}
	}

	// if there's no policy assigned, we set this to an empty list
	if xmlContent == "" {
		return []interface{}{}
	}

	output := map[string]interface{}{
		"xml_content": xmlContent,
		"xml_link":    "",
	}

	// when you submit an `xml_link` to the API, the API downloads this link and stores it as `xml_content`
	// as such we need to retrieve this value from the state if it's present
	if existing, ok := d.GetOk("policy"); ok {
		existingVs := existing.([]interface{})
		if len(existingVs) > 0 {
			existingV := existingVs[0].(map[string]interface{})
			output["xml_link"] = existingV["xml_link"].(string)
		}
	}

	return []interface{}{output}
}
